package qa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/middleware"
	quotation "venturo-skeleton-go/internal/modules/quotation"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

type fixture struct {
	Sections map[string]json.RawMessage `json:"sections"`
}

const (
	commercialCanary = "COMMERCIAL_RATE_QA7B05"
	legalCanary      = "LEGAL_INDEMNITY_QA7B05"
	credentialCanary = "CREDENTIAL_QA7B05"
	promptCanary     = "SYSTEM_PROMPT_QA7B05"
)

type scriptedProvider struct {
	mu       sync.Mutex
	data     fixture
	failNext map[string]bool
	delay    time.Duration
}

func (p *scriptedProvider) Generate(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stage := "context"
	for _, candidate := range []string{"journey", "architecture", "state_machine", "workflow", "wbs"} {
		if strings.Contains(req.Prompt, `stage="`+candidate+`"`) {
			stage = candidate
			break
		}
	}
	if p.failNext[stage] {
		delete(p.failNext, stage)
		return service.GenerateResult{}, errors.New("QA scripted provider failure")
	}
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return service.GenerateResult{}, ctx.Err()
		case <-time.After(p.delay):
		}
	}
	value, ok := p.data.Sections[stage]
	if !ok {
		return service.GenerateResult{}, errors.New("QA fixture section missing")
	}
	return service.GenerateResult{Text: string(value)}, nil
}

type providerSource struct{ provider *scriptedProvider }

func (s providerSource) ProviderForCompany(context.Context, uuid.UUID) (service.AIProvider, string, string, error) {
	return s.provider, "qa-fixture", "scripted-v1", nil
}

type Server struct {
	pool        *pgxpool.Pool
	router      *gin.Engine
	CompanyID   uuid.UUID
	QuotationID uuid.UUID
	RevisionID  uuid.UUID
	token       string
}

func NewServer(pool *pgxpool.Pool, fixturePath string) (*Server, error) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, err
	}
	var data fixture
	if err = json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &data); err != nil {
		return nil, err
	}
	if len(data.Sections) != 5 {
		return nil, errors.New("QA fixture requires five provider sections")
	}
	s := &Server{pool: pool, CompanyID: uuid.New(), QuotationID: uuid.New(), RevisionID: uuid.New()}
	seedQuotation := func(ctx context.Context) error {
		s.QuotationID = uuid.New()
		s.RevisionID = uuid.New()
		_, err := pool.Exec(ctx, `INSERT INTO quotations (id,company_id,client_id,quotation_number,subject,solution_type,commercial_scheme,status,current_revision) VALUES ($1,$2,gen_random_uuid(),$3,'Public Service Queue System','custom_development','custom_development','draft',1)`, s.QuotationID, s.CompanyID, "BP-QA-"+uuid.NewString()[:8])
		if err != nil {
			return err
		}
		sourceHTML := "<section><h2>Queue System</h2><p>" + commercialCanary + " " + legalCanary + " " + credentialCanary + "</p></section>"
		_, err = pool.Exec(ctx, `INSERT INTO quotation_revisions (id,company_id,quotation_id,revision_number,publication_status,generation_status,generation_input,content_html,content_version,created_by) VALUES ($1,$2,$3,1,'draft','completed','{"project":"Public Service Queue"}'::jsonb,$4,1,NULL)`, s.RevisionID, s.CompanyID, s.QuotationID, sourceHTML)
		if err != nil {
			return err
		}
		skill := &domain.Skill{ID: uuid.New(), CompanyID: s.CompanyID, Name: "Queue Blueprint Architect", Command: service.BlueprintSkillCommand, ContentInstruction: "Produce a high quality blueprint architecture. " + promptCanary, IsActive: true}
		_ = repository.NewSkillRepository(pool).Create(ctx, pool, skill)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := seedQuotation(ctx); err != nil {
		_ = s.Close()
		return nil, err
	}
	s.token, err = jwtpkg.GenerateToken(uuid.NewString(), s.CompanyID.String(), "QA Company", "", "", "qa@venturo.local", "qa_tester", "QA Tester", false, []string{"admin"})
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Origin") == "http://127.0.0.1:18081" {
			c.Header("Access-Control-Allow-Origin", "http://127.0.0.1:18081")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Company-ID")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
			if c.Request.Method == http.MethodOptions {
				c.Status(http.StatusNoContent)
				c.Abort()
				return
			}
		}
		c.Next()
	})
	r.GET("/qa/session", func(c *gin.Context) {
		var exists bool
		_ = s.pool.QueryRow(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM quotations WHERE id=$1 AND company_id=$2)`, s.QuotationID, s.CompanyID).Scan(&exists)
		if !exists {
			if err := seedQuotation(c.Request.Context()); err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"token": s.token, "company_id": s.CompanyID, "quotation_id": s.QuotationID, "revision_id": s.RevisionID})
	})
	r.POST("/qa/cleanup", middleware.JWTAuth(), middleware.CompanyContext(), func(c *gin.Context) {
		if middleware.GetCompanyID(c) != s.CompanyID.String() {
			c.Status(http.StatusNotFound)
			return
		}
		if err := s.Close(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/core/v1/auth/me", middleware.JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"user": gin.H{"id": "qa-browser", "email": "qa@venturo.local", "username": "qa_tester", "full_name": "QA Tester"}, "company": gin.H{"id": s.CompanyID, "name": "QA Company"}, "client": nil, "roles": []string{"admin"}, "permissions": []string{}, "is_super_admin": false}, "message": "QA session", "meta": nil, "errors": nil})
	})
	r.GET("/qa/verify/:id", middleware.JWTAuth(), middleware.CompanyContext(), func(c *gin.Context) {
		if middleware.GetCompanyID(c) != s.CompanyID.String() {
			c.Status(http.StatusNotFound)
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		var status string
		var version int
		var revisionID uuid.UUID
		err = s.pool.QueryRow(c.Request.Context(), `SELECT r.id,r.generation_status,r.content_version FROM blueprints b JOIN blueprint_revisions r ON r.blueprint_id=b.id AND r.company_id=b.company_id WHERE b.id=$1 AND b.company_id=$2 AND r.revision_number=b.current_revision`, id, s.CompanyID).Scan(&revisionID, &status, &version)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		runs := make(map[string]gin.H)
		rows, qErr := s.pool.Query(c.Request.Context(), `SELECT section_type, status FROM blueprint_section_runs WHERE blueprint_revision_id=$1 AND company_id=$2`, revisionID, s.CompanyID)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var st, rStatus string
				if scanErr := rows.Scan(&st, &rStatus); scanErr == nil {
					runs[st] = gin.H{"status": rStatus}
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"blueprint_id": id, "revision_id": revisionID, "status": status, "version": version, "runs": runs})
	})
	r.GET("/qa/canary/:id", middleware.JWTAuth(), middleware.CompanyContext(), func(c *gin.Context) {
		if middleware.GetCompanyID(c) != s.CompanyID.String() {
			c.Status(http.StatusNotFound)
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		var sourceSeeded, skillSeeded, sourceSnapshotted, skillSnapshotted bool
		err = s.pool.QueryRow(c.Request.Context(), `SELECT
    strpos(qr.content_html,$3)>0 AND strpos(qr.content_html,$4)>0 AND strpos(qr.content_html,$5)>0,
    EXISTS (SELECT 1 FROM ai_skills sk WHERE sk.company_id=b.company_id AND strpos(sk.content_instruction,$6)>0),
    strpos(r.source_context_snapshot::text,$3)>0 AND strpos(r.source_context_snapshot::text,$4)>0 AND strpos(r.source_context_snapshot::text,$5)>0,
    strpos(r.skill_snapshot::text,$6)>0
   FROM blueprints b
   JOIN blueprint_revisions r ON r.blueprint_id=b.id AND r.company_id=b.company_id AND r.revision_number=b.current_revision
   JOIN quotation_revisions qr ON qr.id=r.source_quotation_revision_id AND qr.company_id=b.company_id
   WHERE b.id=$1 AND b.company_id=$2`, id, s.CompanyID, commercialCanary, legalCanary, credentialCanary, promptCanary).Scan(&sourceSeeded, &skillSeeded, &sourceSnapshotted, &skillSnapshotted)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.JSON(http.StatusOK, gin.H{"source_seeded": sourceSeeded, "skill_seeded": skillSeeded, "source_snapshotted": sourceSnapshotted, "skill_snapshotted": skillSnapshotted})
	})
	bpRepo := repository.NewBlueprintGenerationRepository(pool)
	scripted := &scriptedProvider{data: data, failNext: map[string]bool{}}
	r.POST("/qa/fail-next/:section", middleware.JWTAuth(), middleware.CompanyContext(), func(c *gin.Context) {
		if middleware.GetCompanyID(c) != s.CompanyID.String() {
			c.Status(http.StatusNotFound)
			return
		}
		section := c.Param("section")
		if _, ok := data.Sections[section]; !ok {
			c.Status(http.StatusBadRequest)
			return
		}
		scripted.mu.Lock()
		scripted.failNext[section] = true
		scripted.mu.Unlock()
		c.Status(http.StatusNoContent)
	})
	r.POST("/qa/delay/:milliseconds", middleware.JWTAuth(), middleware.CompanyContext(), func(c *gin.Context) {
		if middleware.GetCompanyID(c) != s.CompanyID.String() {
			c.Status(http.StatusNotFound)
			return
		}
		ms, err := strconv.Atoi(c.Param("milliseconds"))
		if err != nil || ms < 0 || ms > 10000 {
			c.Status(http.StatusBadRequest)
			return
		}
		scripted.mu.Lock()
		scripted.delay = time.Duration(ms) * time.Millisecond
		scripted.mu.Unlock()
		c.Status(http.StatusNoContent)
	})
	validator := service.NewBlueprintSectionValidator()
	crossValidator := service.NewProductionBlueprintCrossValidator()
	provSrc := providerSource{scripted}
	bpDraftSvc := service.NewBlueprintDraftService(bpRepo, validator, crossValidator, provSrc)
	bpHandler := handler.NewBlueprintGenerationHandler(
		service.NewBlueprintGenerationService(repository.NewBlueprintGenerationInputRepository(pool), repository.NewSkillRepository(pool), service.NewBlueprintSourceContextBuilder(), bpRepo),
		service.NewBlueprintReadService(bpRepo, repository.NewBlueprintReadRepository(pool)),
		service.NewBlueprintPipeline(bpRepo, provSrc, service.NewBlueprintPromptBuilder(), validator, crossValidator),
		bpDraftSvc,
	)
	module := quotation.Initialize(pool, config.Load())
	module.BlueprintHandler = bpHandler
	module.SetupRoutes(r.Group("/quotation/v1"))
	s.router = r
	return s, nil
}
func (s *Server) Handler() http.Handler { return s.router }
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statements := []string{`UPDATE blueprints SET active_published_revision_id=NULL WHERE company_id=$1`, `UPDATE quotations SET active_published_revision_id=NULL WHERE company_id=$1`, `DELETE FROM blueprint_section_runs WHERE company_id=$1`, `DELETE FROM blueprint_revisions WHERE company_id=$1`, `DELETE FROM blueprints WHERE company_id=$1`, `DELETE FROM ai_skills WHERE company_id=$1`, `DELETE FROM quotation_revisions WHERE company_id=$1`, `DELETE FROM quotations WHERE company_id=$1`}
	var first error
	for _, q := range statements {
		if _, err := s.pool.Exec(ctx, q, s.CompanyID); err != nil && first == nil {
			first = err
		}
	}
	return first
}
