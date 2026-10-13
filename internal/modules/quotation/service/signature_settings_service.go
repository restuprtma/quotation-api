package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrInvalidPNGFormat = errors.New("image must be a valid PNG data URL")
)

func ValidatePNGDataURL(dataUrl string, allowEmpty bool) error {
	trimmed := strings.TrimSpace(dataUrl)
	if trimmed == "" {
		if allowEmpty {
			return nil
		}
		return ErrInvalidPNGFormat
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(trimmed, prefix) {
		return ErrInvalidPNGFormat
	}
	rawB64 := strings.TrimPrefix(trimmed, prefix)
	decoded, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil || len(decoded) == 0 {
		return ErrInvalidPNGFormat
	}
	pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(decoded) < len(pngMagic) || !bytes.Equal(decoded[:len(pngMagic)], pngMagic) {
		return ErrInvalidPNGFormat
	}
	return nil
}

func (s *Service) GetSignatureSettings(ctx context.Context, companyID string) (*dto.SignatureSettingsResponse, error) {
	cUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, ErrNotFound
	}
	settings, err := s.repo.GetSignatureSettings(ctx, cUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Return default empty unconfigured object
			return &dto.SignatureSettingsResponse{
				DirectorName:  "Yulianto Frandi",
				DirectorTitle: "Direksi Venturo Pro",
				IsConfigured:  false,
			}, nil
		}
		return nil, err
	}

	return &dto.SignatureSettingsResponse{
		DirectorName:          settings.DirectorName,
		DirectorTitle:         settings.DirectorTitle,
		DirectorSignatureData: settings.DirectorSignatureData,
		CompanyStampData:      settings.CompanyStampData,
		IsConfigured:          settings.DirectorSignatureData != "" && settings.CompanyStampData != "",
	}, nil
}

func (s *Service) UpdateSignatureSettings(ctx context.Context, companyID string, req dto.UpdateSignatureSettingsRequest) (*dto.SignatureSettingsResponse, error) {
	cUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, ErrNotFound
	}

	if err := ValidatePNGDataURL(req.DirectorSignatureData, true); err != nil {
		return nil, err
	}
	if err := ValidatePNGDataURL(req.CompanyStampData, true); err != nil {
		return nil, err
	}

	dirName := strings.TrimSpace(req.DirectorName)
	if dirName == "" {
		dirName = "Yulianto Frandi"
	}
	dirTitle := strings.TrimSpace(req.DirectorTitle)
	if dirTitle == "" {
		dirTitle = "Direksi Venturo Pro"
	}

	settings := &domain.SignatureSettings{
		CompanyID:             cUUID,
		DirectorName:          dirName,
		DirectorTitle:         dirTitle,
		DirectorSignatureData: req.DirectorSignatureData,
		CompanyStampData:      req.CompanyStampData,
	}

	res, err := s.repo.UpsertSignatureSettings(ctx, settings)
	if err != nil {
		return nil, err
	}

	return &dto.SignatureSettingsResponse{
		DirectorName:          res.DirectorName,
		DirectorTitle:         res.DirectorTitle,
		DirectorSignatureData: res.DirectorSignatureData,
		CompanyStampData:      res.CompanyStampData,
		IsConfigured:          res.DirectorSignatureData != "" && res.CompanyStampData != "",
	}, nil
}
