package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

var (
	ErrNotFound         = errors.New("quotation not found")
	ErrRevisionNotFound = errors.New("revision not found")
	ErrPublicNotFound   = errors.New("public quotation not found")
	ErrAlreadySigned    = errors.New("quotation is already signed")
)

type Execer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Pool() *pgxpool.Pool {
	return r.db
}

func (r *Repository) Begin(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

func (r *Repository) ClientExists(ctx context.Context, q Execer, clientID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM core.clients WHERE id = $1 AND deleted_at IS NULL)`,
		clientID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) CreateQuotation(ctx context.Context, q Execer, quot *domain.Quotation) error {
	query := `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject, solution_type, commercial_scheme,
			status, current_revision, public_token, active_published_revision_id, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at
	`
	err := q.QueryRow(ctx, query,
		quot.ID, quot.CompanyID, quot.ClientID, quot.QuotationNumber, quot.Subject,
		quot.SolutionType, quot.CommercialScheme, quot.Status, quot.CurrentRevision,
		quot.PublicToken, quot.ActivePublishedRevisionID, quot.CreatedBy,
	).Scan(&quot.CreatedAt, &quot.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create quotation", logger.Err(err))
		return err
	}
	return nil
}

func (r *Repository) GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
	query := `
		SELECT
			q.id, q.company_id, q.client_id, q.quotation_number, q.subject,
			q.solution_type, q.commercial_scheme, q.status, q.current_revision,
			q.public_token, q.active_published_revision_id, q.created_by,
			q.created_at, q.updated_at, q.deleted_at,
			c.name as client_name,
			qr.budget_maximum
		FROM quotations q
		LEFT JOIN core.clients c ON q.client_id = c.id
		LEFT JOIN quotation_revisions qr ON qr.quotation_id = q.id AND qr.revision_number = q.current_revision
		WHERE q.id = $1 AND q.company_id = $2 AND q.deleted_at IS NULL
	`
	return scanQuotation(r.db.QueryRow(ctx, query, id, companyID))
}

func (r *Repository) ListQuotations(
	ctx context.Context,
	companyID, search, status string,
	limit, offset int,
) ([]domain.Quotation, int64, error) {
	baseQuery := `
		FROM quotations q
		LEFT JOIN core.clients c ON q.client_id = c.id
		LEFT JOIN quotation_revisions qr ON qr.quotation_id = q.id AND qr.revision_number = q.current_revision
		WHERE q.company_id = $1 AND q.deleted_at IS NULL
	`
	args := []any{companyID}
	argIdx := 2

	if search != "" {
		baseQuery += fmt.Sprintf(` AND (q.quotation_number ILIKE $%d OR c.name ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if status != "" {
		baseQuery += fmt.Sprintf(` AND q.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*)`+baseQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			q.id, q.company_id, q.client_id, q.quotation_number, q.subject,
			q.solution_type, q.commercial_scheme, q.status, q.current_revision,
			q.public_token, q.active_published_revision_id, q.created_by,
			q.created_at, q.updated_at, q.deleted_at,
			c.name as client_name,
			qr.budget_maximum
	` + baseQuery + fmt.Sprintf(` ORDER BY q.created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []domain.Quotation
	for rows.Next() {
		q, err := scanQuotationRow(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, *q)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (r *Repository) CountQuotations(ctx context.Context, q Execer, companyID string) (int64, error) {
	var total int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM quotations WHERE company_id = $1 AND deleted_at IS NULL`,
		companyID,
	).Scan(&total)
	return total, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanQuotation(scanner rowScanner) (*domain.Quotation, error) {
	q, err := scanQuotationRow(scanner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		logger.Error("Failed to scan quotation", logger.Err(err))
		return nil, err
	}
	return q, nil
}

func scanQuotationRow(scanner rowScanner) (*domain.Quotation, error) {
	q := &domain.Quotation{}
	err := scanner.Scan(
		&q.ID, &q.CompanyID, &q.ClientID, &q.QuotationNumber, &q.Subject,
		&q.SolutionType, &q.CommercialScheme, &q.Status, &q.CurrentRevision,
		&q.PublicToken, &q.ActivePublishedRevisionID, &q.CreatedBy,
		&q.CreatedAt, &q.UpdatedAt, &q.DeletedAt,
		&q.ClientName,
		&q.BudgetMaximum,
	)
	if err != nil {
		return nil, err
	}
	return q, nil
}
