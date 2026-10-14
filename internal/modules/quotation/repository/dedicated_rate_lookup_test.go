package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestDedicatedRateLookup(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for dedicated rate lookup integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	companyID := uuid.New()
	otherCompanyID := uuid.New()
	cardIDs := make(map[string]uuid.UUID)
	officialCodes := []string{"DEFAULT", "VENTURO-STD-2026", "VENTURO-BUMN-2026", "VENTURO-INTL-2026"}
	allCodes := append(append([]string{}, officialCodes...), "CUSTOM-NOT-FOR-GENERATOR")

	for _, code := range allCodes {
		cardID := uuid.New()
		cardIDs[code] = cardID
		_, err = pool.Exec(ctx, `
			INSERT INTO rate_cards (
				id, company_id, code, name, market, client_tier, currency,
				description, short_term_surcharge_pct, enterprise_markup_pct,
				effective_from, is_active
			) VALUES ($1,$2,$3,$3,'national','commercial_national','IDR','test',40,35,$4,true)
		`, cardID, companyID, code, time.Now())
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM rate_cards WHERE company_id IN ($1, $2)`, companyID, otherCompanyID)
	})

	itemID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO rate_card_items (
			id, company_id, rate_card_id, role, display_name, seniority, discipline,
			term_type, amount_per_month, recruitment_fee, onboarding_fee
		) VALUES ($1,$2,$3,'backend_programmer','Back End Programmer','senior','ENGINEERING','long_term',12000000,1000000,500000)
	`, itemID, companyID, cardIDs["VENTURO-STD-2026"])
	require.NoError(t, err)

	repo := repository.NewRateCardRepository(pool)

	t.Run("resolves an exact tenant-scoped item case-insensitively", func(t *testing.T) {
		item, lookupErr := repo.ResolveItem(ctx, companyID.String(), cardIDs["VENTURO-STD-2026"].String(), "BACKEND_PROGRAMMER", "SENIOR", "long_term")
		require.NoError(t, lookupErr)
		assert.Positive(t, item.AmountPerMonth)
		assert.Equal(t, item.RecruitmentFee+item.OnboardingFee, item.TotalHiringFee())
	})

	t.Run("resolves the prompt card with its tenant-owned items at an effective date", func(t *testing.T) {
		card, items, lookupErr := repo.GetActiveRateCardByCode(ctx, companyID.String(), "VENTURO-STD-2026", time.Now())
		require.NoError(t, lookupErr)
		require.NotNil(t, card)
		assert.Equal(t, "VENTURO-STD-2026", card.Code)
		require.Len(t, items, 1)
		assert.Equal(t, companyID, items[0].CompanyID)
		assert.Equal(t, card.ID, items[0].RateCardID)
	})

	t.Run("does not resolve another tenant's card", func(t *testing.T) {
		_, _, lookupErr := repo.GetActiveRateCardByCode(ctx, otherCompanyID.String(), "VENTURO-STD-2026", time.Now())
		assert.ErrorIs(t, lookupErr, repository.ErrRateCardNotFound)
	})

	t.Run("returns a card with no items without inventing roles", func(t *testing.T) {
		card, items, lookupErr := repo.GetActiveRateCardByCode(ctx, companyID.String(), "VENTURO-BUMN-2026", time.Now())
		require.NoError(t, lookupErr)
		assert.Equal(t, "VENTURO-BUMN-2026", card.Code)
		assert.Empty(t, items)
	})

	t.Run("returns the stable missing-item error across tenants", func(t *testing.T) {
		_, lookupErr := repo.ResolveItem(ctx, otherCompanyID.String(), cardIDs["VENTURO-STD-2026"].String(), "backend_programmer", "senior", "long_term")
		assert.ErrorIs(t, lookupErr, dto.ErrMissingRateItem)
	})

	t.Run("rejects an inactive official card", func(t *testing.T) {
		_, updateErr := pool.Exec(ctx, `UPDATE rate_cards SET is_active = false WHERE id = $1 AND company_id = $2`, cardIDs["VENTURO-STD-2026"], companyID)
		require.NoError(t, updateErr)
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `UPDATE rate_cards SET is_active = true WHERE id = $1 AND company_id = $2`, cardIDs["VENTURO-STD-2026"], companyID)
		})

		_, lookupErr := repo.ResolveItem(ctx, companyID.String(), cardIDs["VENTURO-STD-2026"].String(), "backend_programmer", "senior", "long_term")
		assert.ErrorIs(t, lookupErr, dto.ErrMissingRateItem)

		_, updateErr = pool.Exec(ctx, `UPDATE rate_cards SET is_active = true WHERE id = $1 AND company_id = $2`, cardIDs["VENTURO-STD-2026"], companyID)
		require.NoError(t, updateErr)
	})

	t.Run("rejects an expired card at the requested date", func(t *testing.T) {
		_, updateErr := pool.Exec(ctx, `UPDATE rate_cards SET effective_until = $1 WHERE id = $2 AND company_id = $3`, time.Now().Add(-time.Hour), cardIDs["VENTURO-STD-2026"], companyID)
		require.NoError(t, updateErr)
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `UPDATE rate_cards SET effective_until = NULL WHERE id = $1 AND company_id = $2`, cardIDs["VENTURO-STD-2026"], companyID)
		})
		_, _, lookupErr := repo.GetActiveRateCardByCode(ctx, companyID.String(), "VENTURO-STD-2026", time.Now())
		assert.ErrorIs(t, lookupErr, repository.ErrRateCardNotFound)
	})

	t.Run("lists all active generator cards ordered by default", func(t *testing.T) {
		cards, listErr := repo.ListGeneratorCards(ctx, companyID.String())
		require.NoError(t, listErr)
		require.Len(t, cards, 5)
		codes := make([]string, 0, len(cards))
		for _, card := range cards {
			codes = append(codes, card.Code)
		}
		assert.ElementsMatch(t, allCodes, codes)
	})
}
