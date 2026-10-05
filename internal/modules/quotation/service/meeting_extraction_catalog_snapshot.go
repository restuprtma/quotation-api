package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// RevisionRateCardSnapshotLoader reads only the persisted revision snapshot;
// it never consults the live catalog during an attempt.
type RevisionRateCardSnapshotLoader struct{}

func (RevisionRateCardSnapshotLoader) LoadExtractionCatalogSnapshot(_ context.Context, revision *domain.QuotationRevision) (MeetingExtractionCatalogSnapshot, error) {
	if revision == nil {
		return MeetingExtractionCatalogSnapshot{}, fmt.Errorf("revision is required")
	}
	card, items, err := promptRateCardSnapshot(revision.RateCardSnapshot)
	if err != nil {
		return MeetingExtractionCatalogSnapshot{}, err
	}
	hash := sha256.Sum256(revision.RateCardSnapshot)
	return MeetingExtractionCatalogSnapshot{Items: append([]domain.RateCardItem(nil), items...), Hash: hex.EncodeToString(hash[:]), RateCardID: card.ID.String()}, nil
}
