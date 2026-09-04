package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var ErrRateCardNotFound = errors.New("rate card not found")

var generatorRateCardCodes = []string{
	"DEFAULT",
	"VENTURO-STD-2026",
	"VENTURO-BUMN-2026",
	"VENTURO-INTL-2026",
}

type RateCardRepository struct {
	db *pgxpool.Pool
}

func NewRateCardRepository(db *pgxpool.Pool) *RateCardRepository {
	return &RateCardRepository{db: db}
}

func (r *RateCardRepository) Create(ctx context.Context, q Execer, rc *domain.RateCard) error {
	if rc.ClientTier == "" {
		rc.ClientTier = "commercial_national"
	}
	if rc.ShortTermSurchargePct == 0 {
		rc.ShortTermSurchargePct = 40
	}
	if rc.EnterpriseMarkupPct == 0 {
		rc.EnterpriseMarkupPct = 35
	}
	query := `
		INSERT INTO rate_cards (
			id, company_id, code, name, market, client_tier, currency, description,
			short_term_surcharge_pct, enterprise_markup_pct, effective_from, effective_until, is_active, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		rc.ID, rc.CompanyID, rc.Code, rc.Name, rc.Market, rc.ClientTier, rc.Currency, rc.Description,
		rc.ShortTermSurchargePct, rc.EnterpriseMarkupPct, rc.EffectiveFrom, rc.EffectiveUntil, rc.IsActive, rc.CreatedBy,
	).Scan(&rc.CreatedAt, &rc.UpdatedAt)
}

func (r *RateCardRepository) CreateItem(ctx context.Context, q Execer, item *domain.RateCardItem) error {
	query := `
		INSERT INTO rate_card_items (
			id, company_id, rate_card_id, role, seniority, term_type, amount_per_month
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		item.ID, item.CompanyID, item.RateCardID, item.Role, item.Seniority, item.TermType, item.AmountPerMonth,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
}

func (r *RateCardRepository) CreateWithItems(ctx context.Context, q Execer, rc *domain.RateCard, items []*domain.RateCardItem) error {
	if err := r.Create(ctx, q, rc); err != nil {
		return fmt.Errorf("create rate card: %w", err)
	}
	for i, item := range items {
		item.RateCardID = rc.ID
		item.CompanyID = rc.CompanyID
		if err := r.CreateItemExpanded(ctx, q, item); err != nil {
			return fmt.Errorf("create item[%d]: %w", i, err)
		}
	}
	return nil
}

func (r *RateCardRepository) CreateItemExpanded(ctx context.Context, q Execer, item *domain.RateCardItem) error {
	if item.Discipline == "" {
		item.Discipline = domain.DisciplineEngineering
	}
	query := `
		INSERT INTO rate_card_items (
			id, company_id, rate_card_id, role, display_name, seniority, discipline,
			term_type, amount_per_month,
			base_salary, tech_lead_fee, ai_rnd_fee, workspace_fee, management_fee,
			education_requirement, experience_requirement,
			published_rate, max_discount_pct, floor_rate,
			kickoff_fee, recruitment_fee, onboarding_fee
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		item.ID, item.CompanyID, item.RateCardID, item.Role, item.DisplayName,
		item.Seniority, item.Discipline, item.TermType, item.AmountPerMonth,
		item.BaseSalary, item.TechLeadFee, item.AiRndFee, item.WorkspaceFee, item.ManagementFee,
		item.EducationRequirement, item.ExperienceRequirement,
		item.PublishedRate, item.MaxDiscountPct, item.FloorRate,
		item.KickoffFee, item.RecruitmentFee, item.OnboardingFee,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
}

func (r *RateCardRepository) DeleteItemsByRateCard(ctx context.Context, q Execer, rateCardID, companyID string) error {
	query := `DELETE FROM rate_card_items WHERE rate_card_id = $1 AND company_id = $2`
	_, err := q.Exec(ctx, query, rateCardID, companyID)
	return err
}

func (r *RateCardRepository) Get(ctx context.Context, id, companyID string) (*domain.RateCard, error) {
	query := `
		SELECT id, company_id, code, name, market, client_tier, currency, description,
			short_term_surcharge_pct, enterprise_markup_pct,
			effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE id = $1 AND company_id = $2
	`
	rc := &domain.RateCard{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&rc.ID, &rc.CompanyID, &rc.Code, &rc.Name, &rc.Market, &rc.ClientTier, &rc.Currency, &rc.Description,
		&rc.ShortTermSurchargePct, &rc.EnterpriseMarkupPct,
		&rc.EffectiveFrom, &rc.EffectiveUntil, &rc.IsActive, &rc.CreatedBy, &rc.CreatedAt, &rc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRateCardNotFound
	}
	return rc, err
}

func (r *RateCardRepository) GetWithDetails(ctx context.Context, id, companyID string) (*domain.RateCard, []*domain.RateCardItem, error) {
	rc, err := r.Get(ctx, id, companyID)
	if err != nil {
		return nil, nil, err
	}

	itemQuery := `
		SELECT id, company_id, rate_card_id, role, display_name, seniority, discipline,
			term_type, amount_per_month,
			base_salary, tech_lead_fee, ai_rnd_fee, workspace_fee, management_fee,
			education_requirement, experience_requirement,
			published_rate, max_discount_pct, floor_rate,
			kickoff_fee, recruitment_fee, onboarding_fee,
			created_at, updated_at
		FROM rate_card_items
		WHERE rate_card_id = $1 AND company_id = $2
		ORDER BY discipline, role, seniority, term_type
	`
	rows, err := r.db.Query(ctx, itemQuery, id, companyID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]*domain.RateCardItem, 0)
	for rows.Next() {
		item := &domain.RateCardItem{}
		if err := rows.Scan(
			&item.ID, &item.CompanyID, &item.RateCardID, &item.Role, &item.DisplayName,
			&item.Seniority, &item.Discipline, &item.TermType, &item.AmountPerMonth,
			&item.BaseSalary, &item.TechLeadFee, &item.AiRndFee, &item.WorkspaceFee, &item.ManagementFee,
			&item.EducationRequirement, &item.ExperienceRequirement,
			&item.PublishedRate, &item.MaxDiscountPct, &item.FloorRate,
			&item.KickoffFee, &item.RecruitmentFee, &item.OnboardingFee,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}

	return rc, items, rows.Err()
}

func (r *RateCardRepository) ListAll(ctx context.Context, companyID string, activeOnly bool) ([]*domain.RateCard, error) {
	query := `
		SELECT id, company_id, code, name, market, client_tier, currency, description,
			short_term_surcharge_pct, enterprise_markup_pct,
			effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE company_id = $1
	`
	if activeOnly {
		query += ` AND is_active = true`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*domain.RateCard, 0)
	for rows.Next() {
		rc := &domain.RateCard{}
		if err := rows.Scan(
			&rc.ID, &rc.CompanyID, &rc.Code, &rc.Name, &rc.Market, &rc.ClientTier,
			&rc.Currency, &rc.Description, &rc.ShortTermSurchargePct, &rc.EnterpriseMarkupPct,
			&rc.EffectiveFrom, &rc.EffectiveUntil, &rc.IsActive, &rc.CreatedBy,
			&rc.CreatedAt, &rc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, rc)
	}
	return list, rows.Err()
}

func (r *RateCardRepository) Update(ctx context.Context, q Execer, rc *domain.RateCard) error {
	query := `
		UPDATE rate_cards
		SET code = $1, name = $2, market = $3, client_tier = $4, currency = $5, description = $6,
			short_term_surcharge_pct = $7, enterprise_markup_pct = $8,
			effective_from = $9, effective_until = $10, is_active = $11, updated_at = NOW()
		WHERE id = $12 AND company_id = $13
		RETURNING updated_at
	`
	err := q.QueryRow(ctx, query,
		rc.Code, rc.Name, rc.Market, rc.ClientTier, rc.Currency, rc.Description,
		rc.ShortTermSurchargePct, rc.EnterpriseMarkupPct,
		rc.EffectiveFrom, rc.EffectiveUntil, rc.IsActive, rc.ID, rc.CompanyID,
	).Scan(&rc.UpdatedAt)
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
		SELECT id, company_id, code, name, market, client_tier, currency, description,
			short_term_surcharge_pct, enterprise_markup_pct,
			effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE company_id = $1 AND market = $2 AND is_active = true
		ORDER BY effective_from DESC LIMIT 1
	`
	rc := &domain.RateCard{}
	err := r.db.QueryRow(ctx, query, companyID, market).Scan(
		&rc.ID, &rc.CompanyID, &rc.Code, &rc.Name, &rc.Market, &rc.ClientTier, &rc.Currency, &rc.Description,
		&rc.ShortTermSurchargePct, &rc.EnterpriseMarkupPct,
		&rc.EffectiveFrom, &rc.EffectiveUntil, &rc.IsActive, &rc.CreatedBy, &rc.CreatedAt, &rc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRateCardNotFound
	}
	return rc, err
}

func (r *RateCardRepository) ListActiveRateCards(ctx context.Context, companyID string) ([]*domain.RateCard, error) {
	return r.ListAll(ctx, companyID, true)
}

// ResolveItem returns the single tenant-owned rate item matching the selected
// card and commercial dimensions. Role and seniority are case-insensitive to
// tolerate presentation-layer casing, while term type remains an exact enum.
func (r *RateCardRepository) ResolveItem(ctx context.Context, companyID, rateCardID, role, seniority, termType string) (*domain.RateCardItem, error) {
	query := `
		SELECT id, company_id, rate_card_id, role, display_name, seniority, discipline,
			term_type, amount_per_month,
			base_salary, tech_lead_fee, ai_rnd_fee, workspace_fee, management_fee,
			education_requirement, experience_requirement,
			published_rate, max_discount_pct, floor_rate,
			kickoff_fee, recruitment_fee, onboarding_fee,
			created_at, updated_at
		FROM rate_card_items
		WHERE company_id = $1
			AND rate_card_id = $2
			AND LOWER(role) = LOWER($3)
			AND LOWER(seniority) = LOWER($4)
			AND term_type = $5
		LIMIT 2
	`
	rows, err := r.db.Query(ctx, query, companyID, rateCardID, role, seniority, termType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []*domain.RateCardItem
	for rows.Next() {
		item := &domain.RateCardItem{}
		if err := rows.Scan(
			&item.ID, &item.CompanyID, &item.RateCardID, &item.Role, &item.DisplayName,
			&item.Seniority, &item.Discipline, &item.TermType, &item.AmountPerMonth,
			&item.BaseSalary, &item.TechLeadFee, &item.AiRndFee, &item.WorkspaceFee, &item.ManagementFee,
			&item.EducationRequirement, &item.ExperienceRequirement,
			&item.PublishedRate, &item.MaxDiscountPct, &item.FloorRate,
			&item.KickoffFee, &item.RecruitmentFee, &item.OnboardingFee,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		matches = append(matches, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, dto.ErrMissingRateItem
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("duplicate rate items for role %q, seniority %q, term %q", role, seniority, termType)
	}
	return matches[0], nil
}

// ListGeneratorCards exposes only the four official master-card codes to the
// quotation generator. It deliberately retains both tenant and active filters;
// admin CRUD continues to use ListAll.
func (r *RateCardRepository) ListGeneratorCards(ctx context.Context, companyID string) ([]*domain.RateCard, error) {
	query := `
		SELECT id, company_id, code, name, market, client_tier, currency, description,
			short_term_surcharge_pct, enterprise_markup_pct,
			effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE company_id = $1 AND is_active = true AND code = ANY($2)
		ORDER BY CASE code
			WHEN 'DEFAULT' THEN 1
			WHEN 'VENTURO-STD-2026' THEN 2
			WHEN 'VENTURO-BUMN-2026' THEN 3
			WHEN 'VENTURO-INTL-2026' THEN 4
		END
	`
	rows, err := r.db.Query(ctx, query, companyID, generatorRateCardCodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]*domain.RateCard, 0, len(generatorRateCardCodes))
	for rows.Next() {
		card := &domain.RateCard{}
		if err := rows.Scan(
			&card.ID, &card.CompanyID, &card.Code, &card.Name, &card.Market, &card.ClientTier,
			&card.Currency, &card.Description, &card.ShortTermSurchargePct, &card.EnterpriseMarkupPct,
			&card.EffectiveFrom, &card.EffectiveUntil, &card.IsActive, &card.CreatedBy,
			&card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (r *RateCardRepository) GetOvertimeConfigs(ctx context.Context, rateCardID, companyID string) ([]*domain.OvertimeRateConfig, error) {
	query := `
		SELECT id, company_id, rate_card_id, role, seniority, term_type,
			base_monthly_rate, hourly_divisor,
			workday_first_hour_multiplier, workday_subsequent_multiplier,
			holiday_base_multiplier, holiday_9th_hour_multiplier, holiday_10plus_multiplier,
			created_at, updated_at
		FROM overtime_rate_configs
		WHERE rate_card_id = $1 AND company_id = $2
		ORDER BY role, seniority, term_type
	`
	rows, err := r.db.Query(ctx, query, rateCardID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.OvertimeRateConfig
	for rows.Next() {
		cfg := &domain.OvertimeRateConfig{}
		if err := rows.Scan(
			&cfg.ID, &cfg.CompanyID, &cfg.RateCardID, &cfg.Role, &cfg.Seniority, &cfg.TermType,
			&cfg.BaseMonthlyRate, &cfg.HourlyDivisor,
			&cfg.WorkdayFirstHourMultiplier, &cfg.WorkdaySubsequentMultiplier,
			&cfg.HolidayBaseMultiplier, &cfg.Holiday9thHourMultiplier, &cfg.Holiday10PlusMultiplier,
			&cfg.CreatedAt, &cfg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, cfg)
	}
	return list, rows.Err()
}

func (r *RateCardRepository) GetInfrastructurePackages(ctx context.Context, rateCardID, companyID string) ([]*domain.InfrastructurePackage, error) {
	query := `
		SELECT id, company_id, rate_card_id, category, package_name, vcpu, ram_gb, storage_gb, amount_per_month,
			created_at, updated_at
		FROM infrastructure_packages
		WHERE rate_card_id = $1 AND company_id = $2
		ORDER BY category, amount_per_month ASC
	`
	rows, err := r.db.Query(ctx, query, rateCardID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.InfrastructurePackage
	for rows.Next() {
		pkg := &domain.InfrastructurePackage{}
		if err := rows.Scan(
			&pkg.ID, &pkg.CompanyID, &pkg.RateCardID, &pkg.Category, &pkg.PackageName,
			&pkg.VCPU, &pkg.RAMGB, &pkg.StorageGB, &pkg.AmountPerMonth,
			&pkg.CreatedAt, &pkg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, pkg)
	}
	return list, rows.Err()
}
