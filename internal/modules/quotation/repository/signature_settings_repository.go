package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func (r *Repository) GetSignatureSettings(ctx context.Context, companyID uuid.UUID) (*domain.SignatureSettings, error) {
	query := `
		SELECT id, company_id, director_name, director_title, director_signature_data, company_stamp_data, created_at, updated_at
		FROM quotation_signature_settings
		WHERE company_id = $1
	`
	row := r.db.QueryRow(ctx, query, companyID)
	s := &domain.SignatureSettings{}
	err := row.Scan(&s.ID, &s.CompanyID, &s.DirectorName, &s.DirectorTitle, &s.DirectorSignatureData, &s.CompanyStampData, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *Repository) UpsertSignatureSettings(ctx context.Context, s *domain.SignatureSettings) (*domain.SignatureSettings, error) {
	query := `
		INSERT INTO quotation_signature_settings (company_id, director_name, director_title, director_signature_data, company_stamp_data, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (company_id) DO UPDATE
		SET director_name = EXCLUDED.director_name,
		    director_title = EXCLUDED.director_title,
		    director_signature_data = EXCLUDED.director_signature_data,
		    company_stamp_data = EXCLUDED.company_stamp_data,
		    updated_at = NOW()
		RETURNING id, company_id, director_name, director_title, director_signature_data, company_stamp_data, created_at, updated_at
	`
	row := r.db.QueryRow(ctx, query, s.CompanyID, s.DirectorName, s.DirectorTitle, s.DirectorSignatureData, s.CompanyStampData)
	res := &domain.SignatureSettings{}
	err := row.Scan(&res.ID, &res.CompanyID, &res.DirectorName, &res.DirectorTitle, &res.DirectorSignatureData, &res.CompanyStampData, &res.CreatedAt, &res.UpdatedAt)
	return res, err
}
