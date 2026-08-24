package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrRateCardNotFound = errors.New("rate card not found")

type RateCardRepository struct {
	db *pgxpool.Pool
}

func NewRateCardRepository(db *pgxpool.Pool) *RateCardRepository {
	return &RateCardRepository{db: db}
}

func (r *RateCardRepository) Create(ctx context.Context, q Execer, rc *domain.RateCard) error {
	query := `
		INSERT INTO rate_cards (
			id, company_id, code, name, market, currency, effective_from, effective_until, is_active, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		rc.ID, rc.CompanyID, rc.Code, rc.Name, rc.Market, rc.Currency, rc.EffectiveFrom, rc.EffectiveUntil, rc.IsActive, rc.CreatedBy,
	).Scan(&rc.CreatedAt, &rc.UpdatedAt)
}

func (r *RateCardRepository) Get(ctx context.Context, id, companyID string) (*domain.RateCard, error) {
	query := `
		SELECT id, company_id, code, name, market, currency, effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE id = $1 AND company_id = $2
	`
	rc := &domain.RateCard{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&rc.ID, &rc.CompanyID, &rc.Code, &rc.Name, &rc.Market, &rc.Currency, &rc.EffectiveFrom, &rc.EffectiveUntil, &rc.IsActive, &rc.CreatedBy, &rc.CreatedAt, &rc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRateCardNotFound
	}
	return rc, err
}

func (r *RateCardRepository) Update(ctx context.Context, q Execer, rc *domain.RateCard) error {
	query := `
		UPDATE rate_cards
		SET code = $1, name = $2, market = $3, currency = $4, effective_from = $5, effective_until = $6, is_active = $7, updated_at = NOW()
		WHERE id = $8 AND company_id = $9
		RETURNING updated_at
	`
	err := q.QueryRow(ctx, query, rc.Code, rc.Name, rc.Market, rc.Currency, rc.EffectiveFrom, rc.EffectiveUntil, rc.IsActive, rc.ID, rc.CompanyID).Scan(&rc.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRateCardNotFound
	}
	return err
}

func (r *RateCardRepository) Delete(ctx context.Context, q Execer, id, companyID string) error {
	query := `
		DELETE FROM rate_cards
		WHERE id = $1 AND company_id = $2
	`
	tag, err := q.Exec(ctx, query, id, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRateCardNotFound
	}
	return nil
}

func (r *RateCardRepository) GetActiveRateCard(ctx context.Context, companyID, market string) (*domain.RateCard, error) {
	query := `
		SELECT id, company_id, code, name, market, currency, effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE company_id = $1 AND market = $2 AND is_active = true
		ORDER BY effective_from DESC LIMIT 1
	`
	rc := &domain.RateCard{}
	err := r.db.QueryRow(ctx, query, companyID, market).Scan(
		&rc.ID, &rc.CompanyID, &rc.Code, &rc.Name, &rc.Market, &rc.Currency, &rc.EffectiveFrom, &rc.EffectiveUntil, &rc.IsActive, &rc.CreatedBy, &rc.CreatedAt, &rc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRateCardNotFound
	}
	return rc, err
}
