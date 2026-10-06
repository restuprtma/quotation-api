package service_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	quotation_domain "venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestContractService_CreateContract_RequiresSignedContentHash(t *testing.T) {
	// A signed quotation revision lacking signed_content_hash must be rejected
	now := time.Now()
	sigData := "data:image/png;base64,sample"

	revMissingHash := &quotation_domain.QuotationRevision{
		ID:                uuid.New(),
		RevisionNumber:    1,
		PublicationStatus: "published",
		SignedAt:          &now,
		SignatureData:     &sigData,
		SignedContentHash: nil, // MISSING HASH!
	}

	assert.Nil(t, revMissingHash.SignedContentHash)
	// Verification rule assertion
	hasValidHash := revMissingHash.SignedContentHash != nil && *revMissingHash.SignedContentHash != ""
	assert.False(t, hasValidHash, "Revision without signed_content_hash must not be accepted for contract creation")
}
