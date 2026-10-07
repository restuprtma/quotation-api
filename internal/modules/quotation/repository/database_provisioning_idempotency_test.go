package repository_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

// fakeTx provides an in-memory pgx.Tx stub for transaction lifecycle tracking.
type fakeTx struct {
	pgx.Tx
	committed  bool
	rolledBack bool
}

func (f *fakeTx) Commit(ctx context.Context) error {
	f.committed = true
	return nil
}

func (f *fakeTx) Rollback(ctx context.Context) error {
	f.rolledBack = true
	return nil
}

// fakeDBPool provides a minimal DBPool returning fakeTx.
type fakeDBPool struct{}

func (p *fakeDBPool) Begin(ctx context.Context) (pgx.Tx, error) {
	return &fakeTx{}, nil
}

// statefulProvisioningStore simulates a PostgreSQL instance with UNIQUE constraints.
type statefulProvisioningStore struct {
	mu sync.Mutex

	skills         map[string]*domain.Skill         // key: companyID + ":" + command
	rateCards      map[string]*domain.RateCard      // key: companyID + ":" + code
	rateCardItems  map[string]*domain.RateCardItem  // key: companyID:rateCardID:role:seniority:termType
	legalTemplates map[string]*domain.LegalTemplate // key: companyID + ":" + code

	createSkillCalls        int
	createRateCardCalls     int
	createRateCardItemCalls int
	createLegalTmplCalls    int
}

func newProvisioningStore() *statefulProvisioningStore {
	return &statefulProvisioningStore{
		skills:         make(map[string]*domain.Skill),
		rateCards:      make(map[string]*domain.RateCard),
		rateCardItems:  make(map[string]*domain.RateCardItem),
		legalTemplates: make(map[string]*domain.LegalTemplate),
	}
}

// statefulSkillRepo implements service.SkillRepo with unique key enforcement.
type statefulSkillRepo struct {
	store *statefulProvisioningStore
}

func (r *statefulSkillRepo) GetActiveSkillByCommand(ctx context.Context, companyID, command string) (*domain.Skill, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s", companyID, command)
	if sk, ok := r.store.skills[key]; ok && sk.IsActive {
		return sk, nil
	}
	return nil, repository.ErrSkillNotFound
}

func (r *statefulSkillRepo) Create(ctx context.Context, q repository.Execer, skill *domain.Skill) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s", skill.CompanyID.String(), skill.Command)
	if _, exists := r.store.skills[key]; exists {
		return fmt.Errorf("ERROR: duplicate key value violates unique constraint \"uq_skills_active_command\" (Key (company_id, command)=(%s, %s) already exists)", skill.CompanyID, skill.Command)
	}
	r.store.skills[key] = skill
	r.store.createSkillCalls++
	return nil
}

func (r *statefulSkillRepo) Update(ctx context.Context, q repository.Execer, skill *domain.Skill) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s", skill.CompanyID.String(), skill.Command)
	r.store.skills[key] = skill
	return nil
}

// statefulRateCardRepo implements service.RateCardRepo with unique key enforcement.
type statefulRateCardRepo struct {
	store *statefulProvisioningStore
}

func (r *statefulRateCardRepo) GetActiveRateCard(ctx context.Context, companyID, codeOrMarket string) (*domain.RateCard, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	for _, rc := range r.store.rateCards {
		if rc.CompanyID.String() == companyID && (rc.Code == codeOrMarket || rc.Market == codeOrMarket) && rc.IsActive {
			return rc, nil
		}
	}
	return nil, repository.ErrRateCardNotFound
}

func (r *statefulRateCardRepo) Create(ctx context.Context, q repository.Execer, rc *domain.RateCard) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s", rc.CompanyID.String(), rc.Code)
	if _, exists := r.store.rateCards[key]; exists {
		return fmt.Errorf("ERROR: duplicate key value violates unique constraint \"uq_rate_cards_active_code\" (Key (company_id, code)=(%s, %s) already exists)", rc.CompanyID, rc.Code)
	}
	r.store.rateCards[key] = rc
	r.store.createRateCardCalls++
	return nil
}

func (r *statefulRateCardRepo) CreateItem(ctx context.Context, q repository.Execer, item *domain.RateCardItem) error {
	return r.CreateItemExpanded(ctx, q, item)
}

func (r *statefulRateCardRepo) CreateItemExpanded(ctx context.Context, q repository.Execer, item *domain.RateCardItem) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s:%s:%s", item.CompanyID.String(), item.RateCardID.String(), item.Role, item.Seniority, item.TermType)
	if _, exists := r.store.rateCardItems[key]; exists {
		return fmt.Errorf("ERROR: duplicate key value violates unique constraint \"uq_rate_card_items_unique\" (Key (company_id, rate_card_id, role, seniority, term_type)=(%s, %s, %s, %s, %s) already exists)", item.CompanyID, item.RateCardID, item.Role, item.Seniority, item.TermType)
	}
	r.store.rateCardItems[key] = item
	r.store.createRateCardItemCalls++
	return nil
}

// statefulLegalTemplateRepo implements service.LegalTemplateRepo with unique key enforcement.
type statefulLegalTemplateRepo struct {
	store *statefulProvisioningStore
}

func (r *statefulLegalTemplateRepo) GetActiveTemplate(ctx context.Context, companyID, code string) (*domain.LegalTemplate, error) {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s", companyID, code)
	if tmpl, ok := r.store.legalTemplates[key]; ok && tmpl.IsActive {
		return tmpl, nil
	}
	return nil, repository.ErrLegalTemplateNotFound
}

func (r *statefulLegalTemplateRepo) Create(ctx context.Context, q repository.Execer, tmpl *domain.LegalTemplate) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	key := fmt.Sprintf("%s:%s", tmpl.CompanyID.String(), tmpl.Code)
	if _, exists := r.store.legalTemplates[key]; exists {
		return fmt.Errorf("ERROR: duplicate key value violates unique constraint \"uq_legal_templates_versioned\" (Key (company_id, code, version)=(%s, %s, %d) already exists)", tmpl.CompanyID, tmpl.Code, tmpl.Version)
	}
	r.store.legalTemplates[key] = tmpl
	r.store.createLegalTmplCalls++
	return nil
}

func TestProvisioning_IdempotentRunMultipleTimes(t *testing.T) {
	companyID := uuid.New().String()
	ctx := context.Background()

	store := newProvisioningStore()
	skillRepo := &statefulSkillRepo{store: store}
	rcRepo := &statefulRateCardRepo{store: store}
	legalRepo := &statefulLegalTemplateRepo{store: store}
	skillSvc := service.NewSkillService(skillRepo)
	dbPool := &fakeDBPool{}

	settingsSvc := service.NewSettingsService(dbPool, skillRepo, rcRepo, legalRepo, skillSvc)

	// --- Run 1: Initial Provisioning ---
	err1 := settingsSvc.EnsureDefaultAIAssets(ctx, companyID)
	require.NoError(t, err1, "first provisioning execution must succeed")

	// Validate expected created assets
	assert.Equal(t, 2, store.createSkillCalls, "must create exactly 2 default skills (/quotation, /blueprint)")
	assert.Equal(t, 1, store.createRateCardCalls, "must create exactly 1 default rate card (DEFAULT)")
	assert.Equal(t, 84, store.createRateCardItemCalls, "must create 84 default rate card items (14 roles * 3 seniorities * 2 term types)")
	assert.Equal(t, 1, store.createLegalTmplCalls, "must create exactly 1 legal template (QUOTATION_TERMS)")

	assert.Len(t, store.skills, 2)
	assert.Len(t, store.rateCards, 1)
	assert.Len(t, store.rateCardItems, 84)
	assert.Len(t, store.legalTemplates, 1)

	// --- Run 2: Second Execution (Idempotency Check) ---
	err2 := settingsSvc.EnsureDefaultAIAssets(ctx, companyID)
	require.NoError(t, err2, "second provisioning execution must succeed idempotently without duplicate key errors")

	// Ensure no duplicate insertion attempts occurred
	assert.Equal(t, 2, store.createSkillCalls, "second run must not call create on skills")
	assert.Equal(t, 1, store.createRateCardCalls, "second run must not call create on rate cards")
	assert.Equal(t, 84, store.createRateCardItemCalls, "second run must not call create on rate card items")
	assert.Equal(t, 1, store.createLegalTmplCalls, "second run must not call create on legal templates")

	assert.Len(t, store.skills, 2)
	assert.Len(t, store.rateCards, 1)
	assert.Len(t, store.rateCardItems, 84)
	assert.Len(t, store.legalTemplates, 1)

	// --- Run 3: Third Execution (Further Stability Check) ---
	err3 := settingsSvc.EnsureDefaultAIAssets(ctx, companyID)
	require.NoError(t, err3, "third provisioning execution must remain fully idempotent")

	assert.Equal(t, 2, store.createSkillCalls)
	assert.Equal(t, 1, store.createRateCardCalls)
	assert.Equal(t, 84, store.createRateCardItemCalls)
	assert.Equal(t, 1, store.createLegalTmplCalls)

	// --- Verify Uniqueness Violation Behavior ---
	// Directly attempting to insert duplicate assets into the store must fail with constraint errors,
	// proving that the test mock strictly enforces PostgreSQL unique constraint rules.
	compUUID := uuid.MustParse(companyID)

	dupSkillErr := skillRepo.Create(ctx, nil, &domain.Skill{
		CompanyID: compUUID,
		Command:   "/quotation",
	})
	assert.Error(t, dupSkillErr, "direct duplicate skill insertion must trigger unique constraint violation")
	assert.Contains(t, dupSkillErr.Error(), "duplicate key value violates unique constraint")

	dupRCErr := rcRepo.Create(ctx, nil, &domain.RateCard{
		CompanyID: compUUID,
		Code:      "DEFAULT",
	})
	assert.Error(t, dupRCErr, "direct duplicate rate card insertion must trigger unique constraint violation")
	assert.Contains(t, dupRCErr.Error(), "duplicate key value violates unique constraint")

	dupTmplErr := legalRepo.Create(ctx, nil, &domain.LegalTemplate{
		CompanyID: compUUID,
		Code:      "QUOTATION_TERMS",
		Version:   1,
	})
	assert.Error(t, dupTmplErr, "direct duplicate legal template insertion must trigger unique constraint violation")
	assert.Contains(t, dupTmplErr.Error(), "duplicate key value violates unique constraint")

	// --- Live PostgreSQL Execution (if TEST_DATABASE_URL is provided) ---
	if dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")); dsn != "" {
		t.Run("PostgreSQL_LiveDatabase", func(t *testing.T) {
			liveCtx, liveCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer liveCancel()

			pool, err := pgxpool.New(liveCtx, dsn)
			if err != nil {
				t.Skipf("skipping live database test: unable to connect: %v", err)
				return
			}
			defer pool.Close()

			if err := pool.Ping(liveCtx); err != nil {
				t.Skipf("skipping live database test: ping failed: %v", err)
				return
			}

			liveCompanyID := uuid.New().String()
			liveCompanyUUID := uuid.MustParse(liveCompanyID)

			t.Cleanup(func() {
				cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cleanCancel()
				_, _ = pool.Exec(cleanCtx, "DELETE FROM rate_card_items WHERE company_id = $1", liveCompanyUUID)
				_, _ = pool.Exec(cleanCtx, "DELETE FROM rate_cards WHERE company_id = $1", liveCompanyUUID)
				_, _ = pool.Exec(cleanCtx, "DELETE FROM ai_skills WHERE company_id = $1", liveCompanyUUID)
				_, _ = pool.Exec(cleanCtx, "DELETE FROM legal_templates WHERE company_id = $1", liveCompanyUUID)
			})

			liveSkillRepo := repository.NewSkillRepository(pool)
			liveRCRepo := repository.NewRateCardRepository(pool)
			liveLegalRepo := repository.NewLegalTemplateRepository(pool)
			liveSkillSvc := service.NewSkillService(liveSkillRepo)
			liveSettingsSvc := service.NewSettingsService(pool, liveSkillRepo, liveRCRepo, liveLegalRepo, liveSkillSvc)

			// Live Run 1
			err = liveSettingsSvc.EnsureDefaultAIAssets(liveCtx, liveCompanyID)
			require.NoError(t, err, "live db: first execution must succeed")

			// Live Run 2
			err = liveSettingsSvc.EnsureDefaultAIAssets(liveCtx, liveCompanyID)
			require.NoError(t, err, "live db: second execution must succeed idempotently")

			// Live Run 3
			err = liveSettingsSvc.EnsureDefaultAIAssets(liveCtx, liveCompanyID)
			require.NoError(t, err, "live db: third execution must succeed idempotently")

			// Verify row counts in PostgreSQL
			var skillCount int
			err = pool.QueryRow(liveCtx, "SELECT COUNT(*) FROM ai_skills WHERE company_id = $1 AND is_active = true", liveCompanyUUID).Scan(&skillCount)
			require.NoError(t, err)
			assert.Equal(t, 2, skillCount)

			var rcCount int
			err = pool.QueryRow(liveCtx, "SELECT COUNT(*) FROM rate_cards WHERE company_id = $1 AND is_active = true", liveCompanyUUID).Scan(&rcCount)
			require.NoError(t, err)
			assert.Equal(t, 1, rcCount)

			var itemCount int
			err = pool.QueryRow(liveCtx, "SELECT COUNT(*) FROM rate_card_items WHERE company_id = $1", liveCompanyUUID).Scan(&itemCount)
			require.NoError(t, err)
			assert.Equal(t, 84, itemCount)

			var tmplCount int
			err = pool.QueryRow(liveCtx, "SELECT COUNT(*) FROM legal_templates WHERE company_id = $1 AND is_active = true", liveCompanyUUID).Scan(&tmplCount)
			require.NoError(t, err)
			assert.Equal(t, 1, tmplCount)
		})
	}
}

func TestDatabaseProvisioning_MigrationsAndSeedersIntegrity(t *testing.T) {
	root := findProjectRoot(t)

	// 1. Verify migrations directories exist
	modules := []string{"core", "quotation", "contract"}
	for _, mod := range modules {
		dir := filepath.Join(root, "internal", "database", "migrations", mod)
		info, err := os.Stat(dir)
		require.NoError(t, err, "migration directory for %s must exist", mod)
		assert.True(t, info.IsDir(), "%s must be a directory", dir)

		// Check up/down pairs
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		files := make(map[string]bool)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				files[e.Name()] = true
			}
		}

		for f := range files {
			if strings.HasSuffix(f, ".up.sql") {
				downFile := strings.TrimSuffix(f, ".up.sql") + ".down.sql"
				assert.True(t, files[downFile], "missing down migration for %s in module %s", f, mod)
			}
		}
	}

	// 2. Verify seeders integrity
	seederQuotation := filepath.Join(root, "internal", "database", "seeders", "quotation", "001_venturo_master_rate_cards.sql")
	content, err := os.ReadFile(seederQuotation)
	require.NoError(t, err, "master rate cards seeder must exist")
	sqlStr := string(content)

	assert.Contains(t, sqlStr, "rate_cards", "seeder must populate rate_cards")
	assert.Contains(t, sqlStr, "rate_card_items", "seeder must populate rate_card_items")
	assert.Contains(t, sqlStr, "DEFAULT", "seeder must include DEFAULT card")
	assert.Contains(t, sqlStr, "VENTURO-STD-2026", "seeder must include VENTURO-STD-2026 card")
	assert.Contains(t, sqlStr, "VENTURO-INTL-2026", "seeder must include VENTURO-INTL-2026 card")
}
