package repository

import (
	"context"
	"errors"

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

// Execer abstracts over *pgxpool.Pool and pgx.Tx so the same queries can run
// inside a transaction (used by the Create flow) or directly against the pool.
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

// Pool returns the underlying connection pool. Exposed so callers can use the
// pool where an Execer (non-transactional path) is expected.
func (r *Repository) Pool() *pgxpool.Pool {
	return r.db
}

// Begin starts a transaction for multi-statement flows (create quotation +
// initial revision + items).
func (r *Repository) Begin(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

// ClientExists returns true if a non-deleted client with the given id exists.
// Clients live in the core.clients table owned by the core/client module.
func (r *Repository) ClientExists(ctx context.Context, q Execer, clientID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM core.clients WHERE id = $1 AND deleted_at IS NULL)`,
		clientID,
	).Scan(&exists)
	return exists, err
}

// CreateQuotation inserts a new quotation (status draft, revision 1) and
// writes the DB-populated fields back into the domain struct.
func (r *Repository) CreateQuotation(ctx context.Context, q Execer, quot *domain.Quotation) error {
	query := `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, project_type,
			status, current_revision, created_by, subject
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at
	`
	err := q.QueryRow(ctx, query,
		quot.ID, quot.CompanyID, quot.ClientID, quot.QuotationNumber,
		quot.ProjectType, quot.Status, quot.CurrentRevision, quot.CreatedBy,
		quot.Subject,
	).Scan(&quot.CreatedAt, &quot.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create quotation", logger.Err(err))
		return err
	}
	return nil
}

// CreateRevision inserts an initial/next revision. public_token is generated
// by the database (gen_random_uuid()) and populated into the domain struct.
func (r *Repository) CreateRevision(ctx context.Context, q Execer, rev *domain.QuotationRevision) error {
	query := `
		INSERT INTO quotation_revisions (
			company_id, quotation_id, revision_number, budget,
			project_description, scope_of_work, maintenance_info, hosting_info,
			notes, ai_raw_response, created_by,
			quotation_date, valid_until, client_snapshot
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, public_token, created_at, updated_at
	`
	err := q.QueryRow(ctx, query,
		rev.CompanyID, rev.QuotationID, rev.RevisionNumber, rev.Budget,
		rev.ProjectDescription, rev.ScopeOfWork, rev.MaintenanceInfo, rev.HostingInfo,
		rev.Notes, rev.AiRawResponse, rev.CreatedBy,
		rev.QuotationDate, rev.ValidUntil, rev.ClientSnapshot,
	).Scan(&rev.ID, &rev.PublicToken, &rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create quotation revision", logger.Err(err))
		return err
	}
	return nil
}



// GetQuotationByID returns one non-deleted quotation scoped to a company.
func (r *Repository) GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
	query := `
		SELECT q.id, q.company_id, q.client_id, q.quotation_number, q.project_type,
		       q.status, q.current_revision, q.created_by, q.created_at, q.updated_at,
		       q.deleted_at, cl.name AS client_name, q.subject
		FROM quotations q
		LEFT JOIN core.clients cl ON cl.id = q.client_id
		WHERE q.id = $1 AND q.company_id = $2 AND q.deleted_at IS NULL
	`
	return scanQuotation(r.db.QueryRow(ctx, query, id, companyID))
}

// GetActiveRevision returns the current (current_revision) revision of a
// quotation.
func (r *Repository) GetActiveRevision(ctx context.Context, quotationID, companyID string, revisionNumber int) (*domain.QuotationRevision, error) {
	query := `
		SELECT id, company_id, quotation_id, revision_number, public_token, budget,
		       project_description, scope_of_work, maintenance_info, hosting_info,
		       notes, ai_raw_response, signature_image_url, signed_at,
		       signed_ip, created_by, created_at, updated_at,
		       quotation_date, valid_until, client_snapshot
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2 AND revision_number = $3
	`
	return scanRevision(r.db.QueryRow(ctx, query, quotationID, companyID, revisionNumber))
}



// UpdateRevision mutates the edited content of an existing revision.
func (r *Repository) UpdateRevision(
	ctx context.Context,
	q Execer,
	rev *domain.QuotationRevision,
) (*domain.QuotationRevision, error) {
	query := `
		UPDATE quotation_revisions
		SET budget = COALESCE($2, budget),
		    project_description = COALESCE($3, project_description),
		    scope_of_work = COALESCE($4, scope_of_work),
		    maintenance_info = COALESCE($5, maintenance_info),
		    hosting_info = COALESCE($6, hosting_info),
		    notes = COALESCE($7, notes),
		    quotation_date = COALESCE($8, quotation_date),
		    valid_until = COALESCE($9, valid_until),
		    client_snapshot = COALESCE($10, client_snapshot),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, company_id, quotation_id, revision_number, public_token, budget,
		          project_description, scope_of_work, maintenance_info, hosting_info,
		          notes, ai_raw_response, signature_image_url, signed_at,
		          signed_ip, created_by, created_at, updated_at,
		          quotation_date, valid_until, client_snapshot
	`
	return scanRevision(q.QueryRow(ctx, query,
		rev.ID, rev.Budget, rev.ProjectDescription, rev.ScopeOfWork,
		rev.MaintenanceInfo, rev.HostingInfo, rev.Notes,
		rev.QuotationDate, rev.ValidUntil, rev.ClientSnapshot,
	))
}

// ListQuotations returns a paginated list of non-deleted quotations for a
// company, optionally filtered by free-text search (quotation number or
// client name) and status.
func (r *Repository) ListQuotations(
	ctx context.Context,
	companyID, search, status string,
	limit, offset int,
) ([]domain.Quotation, int64, error) {
	selectQuery := `
		SELECT q.id, q.company_id, q.client_id, q.quotation_number, q.project_type,
		       q.status, q.current_revision, q.created_by, q.created_at, q.updated_at,
		       q.deleted_at, cl.name AS client_name, q.subject
		FROM quotations q
		LEFT JOIN core.clients cl ON cl.id = q.client_id
		WHERE q.company_id = $1 AND q.deleted_at IS NULL
		  AND ($2::text = '' OR q.quotation_number ILIKE '%' || $2 || '%' OR cl.name ILIKE '%' || $2 || '%')
		  AND ($3::text = '' OR q.status = $3)
		ORDER BY q.created_at DESC
		LIMIT $4 OFFSET $5
	`

	var total int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM quotations q
		 LEFT JOIN core.clients cl ON cl.id = q.client_id
		 WHERE q.company_id = $1 AND q.deleted_at IS NULL
		   AND ($2::text = '' OR q.quotation_number ILIKE '%' || $2 || '%' OR cl.name ILIKE '%' || $2 || '%')
		   AND ($3::text = '' OR q.status = $3)`,
		companyID, search, status,
	).Scan(&total)
	if err != nil {
		logger.Error("Failed to count quotations", logger.Err(err))
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, selectQuery, companyID, search, status, limit, offset)
	if err != nil {
		logger.Error("Failed to list quotations", logger.Err(err))
		return nil, 0, err
	}
	defer rows.Close()

	var out []domain.Quotation
	for rows.Next() {
		q, err := scanQuotationRow(rows)
		if err != nil {
			logger.Error("Failed to scan quotation row", logger.Err(err))
			return nil, 0, err
		}
		out = append(out, *q)
	}
	return out, total, rows.Err()
}

// CountQuotations returns the total non-deleted quotations for a company.
// Used to derive the next sequential part of the quotation number.
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
	var q domain.Quotation
	err := scanner.Scan(
		&q.ID, &q.CompanyID, &q.ClientID, &q.QuotationNumber, &q.ProjectType,
		&q.Status, &q.CurrentRevision, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt,
		&q.DeletedAt, &q.ClientName, &q.Subject,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func scanRevision(scanner rowScanner) (*domain.QuotationRevision, error) {
	var rev domain.QuotationRevision
	err := scanner.Scan(
		&rev.ID, &rev.CompanyID, &rev.QuotationID, &rev.RevisionNumber, &rev.PublicToken,
		&rev.Budget, &rev.ProjectDescription, &rev.ScopeOfWork, &rev.MaintenanceInfo,
		&rev.HostingInfo, &rev.Notes, &rev.AiRawResponse,
		&rev.SignatureImageURL, &rev.SignedAt, &rev.SignedIP, &rev.CreatedBy,
		&rev.CreatedAt, &rev.UpdatedAt,
		&rev.QuotationDate, &rev.ValidUntil, &rev.ClientSnapshot,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		logger.Error("Failed to scan revision", logger.Err(err))
		return nil, err
	}
	return &rev, nil
}
