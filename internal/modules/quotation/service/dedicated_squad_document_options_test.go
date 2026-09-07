package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestResolveDedicatedSquadDocumentOptionsDefaultsAndCatalog(t *testing.T) {
	got, err := ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{})
	require.NoError(t, err)
	assert.Equal(t, "CTH", got.LetterPrefix)
	assert.Equal(t, domain.TaxExclusivePPN, got.TaxTreatment)
	assert.Equal(t, defaultDocumentClauseCodes, got.ClauseCodes)

	for _, prefix := range []string{"CTH", "VEN", "LANDA", "VENTURO"} {
		got, err := ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{LetterPrefix: prefix})
		require.NoError(t, err)
		assert.Equal(t, prefix, got.LetterPrefix)
	}
	for _, treatment := range []domain.TaxTreatment{domain.TaxExclusivePPN, domain.TaxInclusivePPN} {
		got, err := ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{TaxTreatment: treatment})
		require.NoError(t, err)
		assert.NotEmpty(t, dedicatedTaxClause(got))
	}
	for code := range documentClauses {
		got, err := ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{ClauseCodes: []string{code}})
		require.NoError(t, err)
		assert.Len(t, dedicatedClauses(got), 1)
	}
}

func TestResolveDedicatedSquadDocumentOptionsRejectsUnknownAndCopies(t *testing.T) {
	_, err := ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{LetterPrefix: "RAW"})
	require.Error(t, err)
	_, err = ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{TaxTreatment: "raw"})
	require.Error(t, err)
	_, err = ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{ClauseCodes: []string{"raw"}})
	require.Error(t, err)

	input := domain.DocumentOptionsInput{ClauseCodes: []string{"overtime"}}
	got, err := ResolveDedicatedSquadDocumentOptions(input)
	require.NoError(t, err)
	input.ClauseCodes[0] = "third_party"
	assert.Equal(t, []string{"overtime"}, got.ClauseCodes)
}
