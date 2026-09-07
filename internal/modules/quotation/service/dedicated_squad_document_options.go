package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ResolvedDocumentOptions contains only catalog keys. Legal wording remains
// server-owned in this package and is never accepted from a quotation request.
type ResolvedDocumentOptions struct {
	LetterPrefix string              `json:"letter_prefix"`
	TaxTreatment domain.TaxTreatment `json:"tax_treatment"`
	ClauseCodes  []string            `json:"clause_codes"`
}

var documentPrefixes = map[string]struct{}{
	"CTH": {}, "VEN": {}, "LANDA": {}, "VENTURO": {},
}

var documentTaxClauses = map[domain.TaxTreatment]string{
	domain.TaxExclusivePPN: DedicatedTaxClause,
	domain.TaxInclusivePPN: "Seluruh rincian biaya sudah termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.",
}

var documentClauses = map[string]string{
	"out_of_pocket": "Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.",
	"overtime":      "Percepatan pekerjaan di luar jam kerja reguler (malam hari, Sabtu, Minggu, atau hari libur nasional) dikenakan biaya lembur tersendiri.",
	"onsite":        "Pekerjaan on-site di luar lokasi kerja yang disepakati dilakukan berdasarkan persetujuan tertulis Para Pihak dan dapat dikenakan biaya tambahan.",
	"third_party":   "Lisensi pihak ketiga, credit API AI (OpenAI/Anthropic), Payment Gateway, WhatsApp API / SMS Gateway, domain, dan server cloud di luar paket yang disepakati.",
}

var defaultDocumentClauseCodes = []string{"out_of_pocket", "overtime", "onsite", "third_party"}

// ResolveDedicatedSquadDocumentOptions completes a partial profile choice
// with the legacy private-quotation document choices. Slices are copied so a
// caller cannot mutate the resolved profile after validation.
func ResolveDedicatedSquadDocumentOptions(input domain.DocumentOptionsInput) (ResolvedDocumentOptions, error) {
	resolved := ResolvedDocumentOptions{
		LetterPrefix: "CTH",
		TaxTreatment: domain.TaxExclusivePPN,
		ClauseCodes:  append([]string(nil), defaultDocumentClauseCodes...),
	}
	if input.LetterPrefix != "" {
		if _, ok := documentPrefixes[input.LetterPrefix]; !ok {
			return ResolvedDocumentOptions{}, fmt.Errorf("letter_prefix: unsupported value %q", input.LetterPrefix)
		}
		resolved.LetterPrefix = input.LetterPrefix
	}
	if input.TaxTreatment != "" {
		if _, ok := documentTaxClauses[input.TaxTreatment]; !ok {
			return ResolvedDocumentOptions{}, fmt.Errorf("tax_treatment: unsupported value %q", input.TaxTreatment)
		}
		resolved.TaxTreatment = input.TaxTreatment
	}
	if input.ClauseCodes != nil {
		resolved.ClauseCodes = make([]string, len(input.ClauseCodes))
		for i, code := range input.ClauseCodes {
			if _, ok := documentClauses[code]; !ok {
				return ResolvedDocumentOptions{}, fmt.Errorf("clause_codes[%d]: unsupported value %q", i, code)
			}
			resolved.ClauseCodes[i] = code
		}
	}
	return resolved, nil
}

func dedicatedTaxClause(options ResolvedDocumentOptions) string {
	return documentTaxClauses[options.TaxTreatment]
}

func dedicatedClauses(options ResolvedDocumentOptions) []string {
	clauses := make([]string, 0, len(options.ClauseCodes))
	for _, code := range options.ClauseCodes {
		if clause, ok := documentClauses[code]; ok {
			clauses = append(clauses, clause)
		}
	}
	return clauses
}
