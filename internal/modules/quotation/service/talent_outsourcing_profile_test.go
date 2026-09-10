package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResolveTalentOutsourcingProfile(t *testing.T) {
	p, err := ResolveTalentOutsourcingProfile(talentBPJSInput())
	require.NoError(t, err)
	require.Len(t, p.Sections, 4)
	require.True(t, p.ShowNonPersonnel == false)
	require.Contains(t, p.TaxClauseText, "DPP PPN")
}
