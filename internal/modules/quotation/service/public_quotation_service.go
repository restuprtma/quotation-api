package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrPublicQuotationNotFound = errors.New("public quotation not found")
	ErrConsentRequired         = errors.New("consent is required to sign quotation")
	ErrInvalidSignerName       = errors.New("signer name cannot be empty")
	ErrInvalidSignerEmail      = errors.New("invalid signer email address")
	ErrInvalidSignaturePNG     = errors.New("signature must be a valid non-empty PNG data URL")
	ErrQuotationAlreadySigned  = errors.New("quotation revision has already been signed")
)

const maxSignatureDataUrlLen = 2 * 1024 * 1024 // 2MB cap

func ValidateSignaturePayload(name, email, signatureData string, consent bool) error {
	if !consent {
		return ErrConsentRequired
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" || len(trimmedName) > 100 {
		return ErrInvalidSignerName
	}
	trimmedEmail := strings.TrimSpace(email)
	if _, err := mail.ParseAddress(trimmedEmail); err != nil || len(trimmedEmail) > 255 {
		return ErrInvalidSignerEmail
	}

	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(signatureData, prefix) || len(signatureData) > maxSignatureDataUrlLen {
		return ErrInvalidSignaturePNG
	}

	rawB64 := strings.TrimPrefix(signatureData, prefix)
	decoded, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil || len(decoded) == 0 {
		return ErrInvalidSignaturePNG
	}

	// Validate PNG Magic header: \x89 P N G \r \n \x1a \n
	pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(decoded) < len(pngMagic) || !bytes.Equal(decoded[:len(pngMagic)], pngMagic) {
		return ErrInvalidSignaturePNG
	}

	return nil
}

func ValidateSignaturePayloadStreamlined(signatureData string, consent bool) error {
	if !consent {
		return ErrConsentRequired
	}
	if len(signatureData) > maxSignatureDataUrlLen {
		return ErrInvalidSignaturePNG
	}
	if err := ValidatePNGDataURL(signatureData, false); err != nil {
		return ErrInvalidSignaturePNG
	}
	return nil
}

func ComputeContentHash(contentHTML string) string {
	sum := sha256.Sum256([]byte(contentHTML))
	return hex.EncodeToString(sum[:])
}

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
	tokenUUID, err := uuid.Parse(publicToken)
	if err != nil {
		return nil, ErrPublicQuotationNotFound
	}

	if err := ValidateSignaturePayloadStreamlined(req.SignatureData, req.Consent); err != nil {
		return nil, err
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	quot, rev, err := s.repo.LockAndSignQuotationTx(ctx, tx, tokenUUID, req.SignerName, req.SignerEmail, req.SignatureData, ip, userAgent)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPublicQuotationNotFound
		}
		if errors.Is(err, repository.ErrAlreadySigned) {
			return nil, ErrQuotationAlreadySigned
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
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

	return &dto.PublicQuotationResponse{
		QuotationNumber: quot.QuotationNumber,
		Subject:         quot.Subject,
		Client:          dto.PublicQuotationClient{CompanyName: clientName},
		RevisionNumber:  rev.RevisionNumber,
		ContentHTML:     contentHTML,
		PublishedAt:     publishedAt,
		Signed:          true,
		SignerName:      rev.SignerName,
		SignedAt:        rev.SignedAt,
	}, nil
}
