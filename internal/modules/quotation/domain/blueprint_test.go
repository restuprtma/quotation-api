package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestBlueprintDomainContract(t *testing.T) {
	var version int = domain.BlueprintRevision{}.ContentVersion
	var key *uuid.UUID = domain.BlueprintRevision{}.IdempotencyKey
	assert.Zero(t, version)
	assert.Nil(t, key)
	assert.Equal(t, []domain.BlueprintSectionType{
		domain.BlueprintSectionContext,
		domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}, domain.BlueprintSectionOrder)
	assert.Equal(t, domain.BlueprintSectionType("workflow"), domain.BlueprintSectionWorkflow)
	assert.Equal(t, domain.BlueprintSectionStatus("validated"), domain.BlueprintSectionValidated)
}
