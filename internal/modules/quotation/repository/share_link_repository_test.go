package repository_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestShareLinkRepository_UnitStructs(t *testing.T) {
	repo := repository.NewShareLinkRepository(nil)
	assert.NotNil(t, repo)

	linkWithRev := repository.ShareLinkWithRevision{
		ShareLink: domain.ShareLink{
			ID:          uuid.New(),
			CompanyID:   uuid.New(),
			BlueprintID: uuid.New(),
			Label:       "Test Label",
		},
		RevisionNumber: 3,
	}

	assert.Equal(t, 3, linkWithRev.RevisionNumber)
	assert.Equal(t, "Test Label", linkWithRev.Label)
}
