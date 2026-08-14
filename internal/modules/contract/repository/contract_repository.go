package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/contract/domain"
)

type ContractRepository interface {
	Create(ctx context.Context, param domain.CreateContractParam) (*domain.Contract, error)
	GetByID(ctx context.Context, id string, companyID string) (*domain.Contract, error)
	GetByQuotationID(ctx context.Context, quotationID string, companyID string) (*domain.Contract, error)
	GetByPublicToken(ctx context.Context, token string) (*domain.Contract, error)
	Update(ctx context.Context, id string, companyID string, param domain.UpdateContractParam) error
	UpdateStatus(ctx context.Context, id string, companyID string, status domain.ContractStatus) error
	Sign(ctx context.Context, token string, signatureURL string, ip string) error
	ListByCompany(ctx context.Context, companyID string) ([]domain.Contract, error)
}

type contractRepository struct {
	db *pgxpool.Pool
}

func NewContractRepository(db *pgxpool.Pool) ContractRepository {
	return &contractRepository{db: db}
}

func (r *contractRepository) Create(ctx context.Context, param domain.CreateContractParam) (*domain.Contract, error) {
	query := `
		INSERT INTO contracts (
			company_id, quotation_id, revision_id, contract_number, status, client_snapshot, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		) RETURNING id, public_token, created_at, updated_at
	`
	var contract domain.Contract
	contract.CompanyID = param.CompanyID
	contract.QuotationID = param.QuotationID
	contract.RevisionID = param.RevisionID
	contract.ContractNumber = param.ContractNumber
	contract.Status = domain.ContractStatusDraft
	contract.ClientSnapshot = param.ClientSnapshot
	contract.CreatedBy = param.CreatedBy

	err := r.db.QueryRow(ctx, query,
		param.CompanyID,
		param.QuotationID,
		param.RevisionID,
		param.ContractNumber,
		contract.Status,
		param.ClientSnapshot,
		param.CreatedBy,
	).Scan(
		&contract.ID,
		&contract.PublicToken,
		&contract.CreatedAt,
		&contract.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &contract, nil
}

func (r *contractRepository) GetByID(ctx context.Context, id string, companyID string) (*domain.Contract, error) {
	query := `
		SELECT id, company_id, quotation_id, revision_id, contract_number, public_token, status,
		       client_snapshot, scope_data, deliverables_data, commercial_terms, payment_terms,
		       timeline_data, additional_terms, signature_image_url, signed_at, signed_ip, created_by,
		       created_at, updated_at
		FROM contracts
		WHERE id = $1 AND company_id = $2 AND deleted_at IS NULL
	`

	return r.scanContract(r.db.QueryRow(ctx, query, id, companyID))
}

func (r *contractRepository) GetByQuotationID(ctx context.Context, quotationID string, companyID string) (*domain.Contract, error) {
	query := `
		SELECT id, company_id, quotation_id, revision_id, contract_number, public_token, status,
		       client_snapshot, scope_data, deliverables_data, commercial_terms, payment_terms,
		       timeline_data, additional_terms, signature_image_url, signed_at, signed_ip, created_by,
		       created_at, updated_at
		FROM contracts
		WHERE quotation_id = $1 AND company_id = $2 AND deleted_at IS NULL
	`

	return r.scanContract(r.db.QueryRow(ctx, query, quotationID, companyID))
}

func (r *contractRepository) GetByPublicToken(ctx context.Context, token string) (*domain.Contract, error) {
	query := `
		SELECT id, company_id, quotation_id, revision_id, contract_number, public_token, status,
		       client_snapshot, scope_data, deliverables_data, commercial_terms, payment_terms,
		       timeline_data, additional_terms, signature_image_url, signed_at, signed_ip, created_by,
		       created_at, updated_at
		FROM contracts
		WHERE public_token = $1 AND deleted_at IS NULL
	`

	return r.scanContract(r.db.QueryRow(ctx, query, token))
}

func (r *contractRepository) Update(ctx context.Context, id string, companyID string, param domain.UpdateContractParam) error {
	query := `
		UPDATE contracts
		SET scope_data = $1,
		    deliverables_data = $2,
		    commercial_terms = $3,
		    payment_terms = $4,
		    timeline_data = $5,
		    additional_terms = $6,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $7 AND company_id = $8 AND status = 'draft' AND deleted_at IS NULL
	`
	_, err := r.db.Exec(ctx, query,
		param.ScopeData,
		param.DeliverablesData,
		param.CommercialTerms,
		param.PaymentTerms,
		param.TimelineData,
		param.AdditionalTerms,
		id,
		companyID,
	)
	return err
}

func (r *contractRepository) UpdateStatus(ctx context.Context, id string, companyID string, status domain.ContractStatus) error {
	query := `
		UPDATE contracts
		SET status = $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND company_id = $3 AND deleted_at IS NULL
	`
	_, err := r.db.Exec(ctx, query, status, id, companyID)
	return err
}

func (r *contractRepository) Sign(ctx context.Context, token string, signatureURL string, ip string) error {
	query := `
		UPDATE contracts
		SET status = 'signed',
		    signature_image_url = $1,
		    signed_ip = $2,
		    signed_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE public_token = $3 AND status = 'shared' AND deleted_at IS NULL
	`
	_, err := r.db.Exec(ctx, query, signatureURL, ip, token)
	return err
}

func (r *contractRepository) ListByCompany(ctx context.Context, companyID string) ([]domain.Contract, error) {
	query := `
		SELECT id, company_id, quotation_id, revision_id, contract_number, public_token, status,
		       client_snapshot, scope_data, deliverables_data, commercial_terms, payment_terms,
		       timeline_data, additional_terms, signature_image_url, signed_at, signed_ip, created_by,
		       created_at, updated_at
		FROM contracts
		WHERE company_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []domain.Contract
	for rows.Next() {
		contract, err := r.scanContractRow(rows)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, *contract)
	}

	return contracts, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *contractRepository) scanContract(row scanner) (*domain.Contract, error) {
	var c domain.Contract
	err := row.Scan(
		&c.ID, &c.CompanyID, &c.QuotationID, &c.RevisionID, &c.ContractNumber, &c.PublicToken, &c.Status,
		&c.ClientSnapshot, &c.ScopeData, &c.DeliverablesData, &c.CommercialTerms, &c.PaymentTerms,
		&c.TimelineData, &c.AdditionalTerms, &c.SignatureImageURL, &c.SignedAt, &c.SignedIP, &c.CreatedBy,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *contractRepository) scanContractRow(row scanner) (*domain.Contract, error) {
	return r.scanContract(row)
}
