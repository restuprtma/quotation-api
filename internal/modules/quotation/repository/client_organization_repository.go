package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	// ErrClientOrganizationDuplicate indicates that an organization with the same normalized name already exists in the company.
	ErrClientOrganizationDuplicate = errors.New("client organization already exists")
	// ErrClientOrganizationNotFound indicates that the requested client organization was not found.
	ErrClientOrganizationNotFound = errors.New("client organization not found")
)

type ClientOrganizationRepository struct {
	db *pgxpool.Pool
}

func NewClientOrganizationRepository(db *pgxpool.Pool) *ClientOrganizationRepository {
	return &ClientOrganizationRepository{db: db}
}

// Create inserts a new client organization.
func (r *ClientOrganizationRepository) Create(ctx context.Context, org *domain.ClientOrganization) error {
	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	now := time.Now().UTC()
	if org.CreatedAt.IsZero() {
		org.CreatedAt = now
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = now
	}

	query := `
		INSERT INTO quotation_client_organizations (
			company_id, id, name, normalized_name, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		org.CompanyID, org.ID, org.Name, org.NormalizedName, org.CreatedBy, org.CreatedAt, org.UpdatedAt,
	).Scan(&org.CreatedAt, &org.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23505" || strings.Contains(pgErr.ConstraintName, "uq_quotation_client_org_active_name")) {
			return ErrClientOrganizationDuplicate
		}
		return err
	}

	return nil
}

// List returns all active client organizations for a specific company ordered by name ASC.
func (r *ClientOrganizationRepository) List(ctx context.Context, companyID uuid.UUID) ([]*domain.ClientOrganization, error) {
	query := `
		SELECT company_id, id, name, normalized_name, created_by, created_at, updated_at
		FROM quotation_client_organizations
		WHERE company_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
	`

	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.ClientOrganization
	for rows.Next() {
		var org domain.ClientOrganization
		if err := rows.Scan(
			&org.CompanyID,
			&org.ID,
			&org.Name,
			&org.NormalizedName,
			&org.CreatedBy,
			&org.CreatedAt,
			&org.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &org)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if results == nil {
		results = make([]*domain.ClientOrganization, 0)
	}

	return results, nil
}

// FindByNormalizedName looks up an active client organization by its normalized name within a company.
func (r *ClientOrganizationRepository) FindByNormalizedName(ctx context.Context, companyID uuid.UUID, normalizedName string) (*domain.ClientOrganization, error) {
	query := `
		SELECT company_id, id, name, normalized_name, created_by, created_at, updated_at
		FROM quotation_client_organizations
		WHERE company_id = $1 AND normalized_name = $2 AND deleted_at IS NULL
		LIMIT 1
	`

	var org domain.ClientOrganization
	err := r.db.QueryRow(ctx, query, companyID, normalizedName).Scan(
		&org.CompanyID,
		&org.ID,
		&org.Name,
		&org.NormalizedName,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClientOrganizationNotFound
		}
		return nil, err
	}

	return &org, nil
}

// FindByID looks up an active client organization by its ID within a company.
func (r *ClientOrganizationRepository) FindByID(ctx context.Context, companyID uuid.UUID, id uuid.UUID) (*domain.ClientOrganization, error) {
	query := `
		SELECT company_id, id, name, normalized_name, created_by, created_at, updated_at
		FROM quotation_client_organizations
		WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL
		LIMIT 1
	`

	var org domain.ClientOrganization
	err := r.db.QueryRow(ctx, query, companyID, id).Scan(
		&org.CompanyID,
		&org.ID,
		&org.Name,
		&org.NormalizedName,
		&org.CreatedBy,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClientOrganizationNotFound
		}
		return nil, err
	}

	return &org, nil
}
