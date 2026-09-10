package service

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestTalentOutsourcingIntegrity(t *testing.T) {
	in := talentBPJSInput()
	c, _ := CalculateTalentOutsourcing(in)
	p, _ := ResolveTalentOutsourcingProfile(in)
	doc, err := NewTalentOutsourcingRenderer().Render(TalentOutsourcingDocumentInput{Input: in, Calculation: c, Profile: p})
	require.NoError(t, err)
	e := TalentOutsourcingIntegrityExpectations{Input: in, Calculation: c, Profile: p}
	require.NoError(t, ValidateTalentOutsourcingIntegrity(doc, e))
	require.Error(t, ValidateTalentOutsourcingIntegrity(strings.Replace(doc, `data-value="44400000"`, `data-value="1"`, 1), e))
}
