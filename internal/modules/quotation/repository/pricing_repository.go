package repository

import (
	"context"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

// CreatePricingSection inserts a new pricing section.
func (r *Repository) CreatePricingSection(ctx context.Context, q Execer, sec *domain.QuotationPricingSection) error {
	query := `
		INSERT INTO quotation_pricing_sections (
			id, company_id, revision_id, section_name, section_type, description, sort_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	err := q.QueryRow(ctx, query,
		sec.ID, sec.CompanyID, sec.RevisionID, sec.SectionName, sec.SectionType, sec.Description, sec.SortOrder,
	).Scan(&sec.CreatedAt, &sec.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create pricing section", logger.Err(err))
		return err
	}
	return nil
}

// CreatePricingItem inserts a new pricing item.
func (r *Repository) CreatePricingItem(ctx context.Context, q Execer, item *domain.QuotationPricingItem) error {
	query := `
		INSERT INTO quotation_pricing_items (
			id, company_id, section_id, item_name, description, features,
			pricing_mode, unit_label, unit_price, quantity, employee_count,
			duration, duration_unit, billing_period, line_total, manual_override_reason, sort_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING created_at, updated_at
	`
	err := q.QueryRow(ctx, query,
		item.ID, item.CompanyID, item.SectionID, item.ItemName, item.Description, item.Features,
		item.PricingMode, item.UnitLabel, item.UnitPrice, item.Quantity, item.EmployeeCount,
		item.Duration, item.DurationUnit, item.BillingPeriod, item.LineTotal, item.ManualOverrideReason, item.SortOrder,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create pricing item", logger.Err(err))
		return err
	}
	return nil
}

// GetPricingSectionsByRevisionID lists pricing sections sorted by sort_order.
func (r *Repository) GetPricingSectionsByRevisionID(ctx context.Context, q Execer, revisionID string) ([]domain.QuotationPricingSection, error) {
	query := `
		SELECT id, company_id, revision_id, section_name, section_type, description, sort_order, created_at, updated_at
		FROM quotation_pricing_sections
		WHERE revision_id = $1
		ORDER BY sort_order ASC, created_at ASC
	`
	rows, err := q.Query(ctx, query, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.QuotationPricingSection
	for rows.Next() {
		var sec domain.QuotationPricingSection
		if err := rows.Scan(
			&sec.ID, &sec.CompanyID, &sec.RevisionID, &sec.SectionName, &sec.SectionType,
			&sec.Description, &sec.SortOrder, &sec.CreatedAt, &sec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, rows.Err()
}

// GetPricingItemsBySectionID lists pricing items sorted by sort_order.
func (r *Repository) GetPricingItemsBySectionID(ctx context.Context, q Execer, sectionID string) ([]domain.QuotationPricingItem, error) {
	query := `
		SELECT id, company_id, section_id, item_name, description, features,
		       pricing_mode, unit_label, unit_price, quantity, employee_count,
		       duration, duration_unit, billing_period, line_total, manual_override_reason, sort_order,
		       created_at, updated_at
		FROM quotation_pricing_items
		WHERE section_id = $1
		ORDER BY sort_order ASC, created_at ASC
	`
	rows, err := q.Query(ctx, query, sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.QuotationPricingItem
	for rows.Next() {
		var it domain.QuotationPricingItem
		if err := rows.Scan(
			&it.ID, &it.CompanyID, &it.SectionID, &it.ItemName, &it.Description, &it.Features,
			&it.PricingMode, &it.UnitLabel, &it.UnitPrice, &it.Quantity, &it.EmployeeCount,
			&it.Duration, &it.DurationUnit, &it.BillingPeriod, &it.LineTotal, &it.ManualOverrideReason, &it.SortOrder,
			&it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// DeletePricingSectionsByRevisionID deletes all pricing sections (and cascaded items) for a revision.
func (r *Repository) DeletePricingSectionsByRevisionID(ctx context.Context, q Execer, revisionID string) error {
	_, err := q.Exec(ctx, `DELETE FROM quotation_pricing_sections WHERE revision_id = $1`, revisionID)
	return err
}
