package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTalentOutsourcingRenderer(t *testing.T) {
	in := talentBPJSInput()
	c, err := CalculateTalentOutsourcing(in)
	require.NoError(t, err)
	p, err := ResolveTalentOutsourcingProfile(in)
	require.NoError(t, err)
	doc, err := NewTalentOutsourcingRenderer().Render(TalentOutsourcingDocumentInput{Metadata: QuotationMetadata{QuotationNumber: in.DocumentNumber, Subject: in.ServiceName}, Input: in, Calculation: c, Profile: p})
	require.NoError(t, err)
	assertCanonicalOfficialHeader(t, doc, "PENAWARAN LAYANAN TALENT OUTSOURCING")
	require.Contains(t, doc, `data-service="talent_outsourcing"`)
	require.Contains(t, doc, `data-field="grand_total" data-value="44400000"`)
}
