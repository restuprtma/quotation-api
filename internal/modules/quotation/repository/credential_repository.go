package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrCredentialNotFound = errors.New("credential not found")

type CredentialRepository struct {
	db *pgxpool.Pool
}

func NewCredentialRepository(db *pgxpool.Pool) *CredentialRepository {
	return &CredentialRepository{db: db}
}

func (r *CredentialRepository) Create(ctx context.Context, q Execer, cred *domain.Credential) error {
	query := `
		INSERT INTO ai_credentials (
			id, company_id, provider, model, encrypted_api_key, key_hint, is_active, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		cred.ID, cred.CompanyID, cred.Provider, cred.Model, cred.EncryptedAPIKey, cred.KeyHint, cred.IsActive, cred.CreatedBy,
	).Scan(&cred.CreatedAt, &cred.UpdatedAt)
}

func (r *CredentialRepository) Get(ctx context.Context, id, companyID string) (*domain.Credential, error) {
	query := `
		SELECT id, company_id, provider, model, encrypted_api_key, key_hint, is_active, created_by, created_at, updated_at
		FROM ai_credentials
		WHERE id = $1 AND company_id = $2
	`
	cred := &domain.Credential{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&cred.ID, &cred.CompanyID, &cred.Provider, &cred.Model, &cred.EncryptedAPIKey, &cred.KeyHint, &cred.IsActive, &cred.CreatedBy, &cred.CreatedAt, &cred.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCredentialNotFound
	}
	return cred, err
}

func (r *CredentialRepository) Update(ctx context.Context, q Execer, cred *domain.Credential) error {
	query := `
		UPDATE ai_credentials
		SET provider = $1, model = $2, encrypted_api_key = $3, key_hint = $4, is_active = $5, updated_at = NOW()
		WHERE id = $6 AND company_id = $7
		RETURNING updated_at
	`
	err := q.QueryRow(ctx, query, cred.Provider, cred.Model, cred.EncryptedAPIKey, cred.KeyHint, cred.IsActive, cred.ID, cred.CompanyID).Scan(&cred.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCredentialNotFound
	}
	return err
}

func (r *CredentialRepository) Delete(ctx context.Context, q Execer, id, companyID string) error {
	query := `
		DELETE FROM ai_credentials
		WHERE id = $1 AND company_id = $2
	`
	tag, err := q.Exec(ctx, query, id, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

func (r *CredentialRepository) GetActiveCredential(ctx context.Context, companyID string) (*domain.Credential, error) {
	query := `
		SELECT id, company_id, provider, model, encrypted_api_key, key_hint, is_active, created_by, created_at, updated_at
		FROM ai_credentials
		WHERE company_id = $1 AND is_active = true
		LIMIT 1
	`
	cred := &domain.Credential{}
	err := r.db.QueryRow(ctx, query, companyID).Scan(
		&cred.ID, &cred.CompanyID, &cred.Provider, &cred.Model, &cred.EncryptedAPIKey, &cred.KeyHint, &cred.IsActive, &cred.CreatedBy, &cred.CreatedAt, &cred.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCredentialNotFound
	}
	return cred, err
}
