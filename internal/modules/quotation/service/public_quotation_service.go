package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrPublicQuotationNotFound = errors.New("public quotation not found")
)

func (s *Service) GetPublicQuotation(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error) {
	tokenUUID, err := uuid.Parse(publicToken)
	if err != nil {
		return nil, ErrPublicQuotationNotFound
	}

	quot, rev, err := s.repo.GetPublicQuotationByToken(ctx, tokenUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPublicQuotationNotFound
		}
		return nil, err
	}

	clientName := ""
	if quot.ClientName != nil {
		clientName = *quot.ClientName
	}

	contentHTML := ""
	if rev.ContentHTML != nil {
		contentHTML = *rev.ContentHTML
	}

	publishedAt := time.Time{}
	if rev.PublishedAt != nil {
		publishedAt = *rev.PublishedAt
	}

	isSigned := rev.SignedAt != nil && rev.SignatureData != nil

	var signerName *string
	var signedAt *time.Time
	if isSigned {
		signerName = rev.SignerName
		signedAt = rev.SignedAt
	}

	return &dto.PublicQuotationResponse{
		QuotationNumber: quot.QuotationNumber,
		Subject:         quot.Subject,
		Client: dto.PublicQuotationClient{
			CompanyName: clientName,
		},
		RevisionNumber: rev.RevisionNumber,
		ContentHTML:    contentHTML,
		PublishedAt:    publishedAt,
		Signed:         isSigned,
		SignerName:     signerName,
		SignedAt:       signedAt,
	}, nil
}

func (s *Service) SignPublicQuotation(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error) {
	return nil, nil
}
