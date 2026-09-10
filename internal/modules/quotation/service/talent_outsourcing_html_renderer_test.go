package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTalentOutsourcingRenderer(t *testing.T) {
	in := talentBPJSInput()
	c, err := CalculateTalentOutsourcing(in)
	require.NoError(t, err)
	p, err := ResolveTalentOutsourcingProfile(in)
	require.NoError(t, err)
	doc, err := NewTalentOutsourcingRenderer().Render(TalentOutsourcingDocumentInput{Input: in, Calculation: c, Profile: p})
	require.NoError(t, err)
	require.Contains(t, doc, `data-service="talent_outsourcing"`)
	require.Contains(t, doc, `data-field="grand_total" data-value="44400000"`)
}
