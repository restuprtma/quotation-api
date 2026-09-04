package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidRolesContains14Entries(t *testing.T) {
	assert.Len(t, validRoles, 14)
	expected := []string{
		"business_analyst", "system_analyst", "frontend_programmer", "backend_programmer",
		"fullstack_developer", "mobile_developer", "devops", "data_engineer",
		"data_analyst", "ui_ux_designer", "automated_tester", "functional_tester",
		"technical_writer", "pentester",
	}
	for _, role := range expected {
		assert.Contains(t, validRoles, role)
	}
}

func TestRoleAliasResolution(t *testing.T) {
	assert.Equal(t, "backend_programmer", resolveRole("programmer"))
	assert.Equal(t, "automated_tester", resolveRole("quality_assurance"))
	assert.Equal(t, "automated_tester", resolveRole("qa"))
	assert.Equal(t, "devops", resolveRole("devops")) // no alias, unchanged
}
