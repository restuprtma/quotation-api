package service

import (
	"context"
	"fmt"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

// AIProvider generates quotation draft content (description, scope of work,
// blueprint) from the project context submitted by the user. Swap this
// interface for a real LLM provider (e.g. Google Gemini) in a later phase —
// see PRD §7 for the contract.
type AIProvider interface {
	GenerateDraft(ctx context.Context, req *dto.AIGenerateRequest) (*dto.AIGenerateResponse, error)
}

// MockAIProvider returns deterministic dummy content built from the request so
// the frontend can be developed and tested without an LLM API key.
type MockAIProvider struct{}

func (m *MockAIProvider) GenerateDraft(_ context.Context, req *dto.AIGenerateRequest) (*dto.AIGenerateResponse, error) {
	return buildMockDraft(req), nil
}

func buildMockDraft(req *dto.AIGenerateRequest) *dto.AIGenerateResponse {
	projectLabel := projectTypeLabel(req.ProjectType)
	reqs := bulletList(req.ClientRequirements)

	narrative := dto.AINarrative{
		Introduction: fmt.Sprintf(
			"Proyek ini merupakan pengembangan %s untuk %s (industri %s). "+
				"Sistem akan dirancang agar seluruh proses berjalan otomatis, terintegrasi, dan mudah dioperasikan. "+
				"Subject: %s\n\nKebutuhan utama yang menjadi acuan pembangunan:\n%s",
			projectLabel, req.ClientName, req.ClientIndustry, req.Subject, reqs,
		),
		Scope: req.ClientRequirements,
	}

	detail := dto.AIProductOrProjectDetail{}
	if req.ProjectType == "product_based" {
		detail.Mode = "product"
		detail.Modules = []dto.AIModuleDetail{
			{Name: "Core", Features: []string{"Dashboard", "Authentication"}},
			{Name: "Integration", Features: []string{"API Gateway"}},
		}
	} else {
		detail.Mode = "project"
		detail.Goals = []string{"Automation", "Efficiency"}
		detail.Deliverables = []string{"Source code", "Documentation"}
		detail.Milestones = []map[string]interface{}{
			{"name": "Discovery", "duration": "2 weeks"},
			{"name": "Development", "duration": "4 weeks"},
		}
		detail.Team = []map[string]interface{}{
			{"role": "Project Manager", "count": 1},
			{"role": "Developer", "count": 2},
		}
	}

	return &dto.AIGenerateResponse{
		SchemaVersion:          "1.0",
		Narrative:              narrative,
		ProductOrProjectDetail: detail,
		Blueprint:              buildMockBlueprint(req),
		PricingLabels:          []string{"One-time Setup", "Monthly Subscription"},
	}
}

func buildMockBlueprint(req *dto.AIGenerateRequest) dto.AIBlueprint {
	reqs := req.ClientRequirements
	columns := []map[string]interface{}{}

	if len(reqs) > 0 {
		for i, r := range reqs {
			columns = append(columns, map[string]interface{}{
				"id":          fmt.Sprintf("phase-%d", i+1),
				"phase_name":  fmt.Sprintf("Tahap %d: %s", i+1, r),
				"order":       i + 1,
				"description": fmt.Sprintf("Implementasi kebutuhan \"%s\" mulai dari perencanaan hingga operasional.", r),
			})
		}
	}

	diagrams := []dto.AIDiagram{
		{
			Type:    "journey",
			Title:   "User journey",
			Columns: columns,
		},
	}

	return dto.AIBlueprint{
		Title: "Solution Blueprint",
		Summary: fmt.Sprintf(
			"Alur operasional sistem %s untuk %s.",
			projectTypeLabel(req.ProjectType), req.ClientName,
		),
		Actors: []map[string]interface{}{
			{
				"id":          "user",
				"label":       "Pengguna",
				"description": "Pengguna akhir yang menggunakan sistem setiap hari",
			},
			{
				"id":          "admin",
				"label":       "Administrator",
				"description": "Tim internal yang mengelola dan memantau sistem",
			},
		},
		Diagrams:    diagrams,
		Rules:       []string{"Semua data harus divalidasi"},
		Assumptions: []string{"Klien menyediakan API"},
		EdgeCases:   []string{"Jaringan terputus"},
		Outputs:     []string{"Laporan bulanan"},
	}
}

func projectTypeLabel(projectType string) string {
	switch projectType {
	case "product_based":
		return "produk jadi"
	case "project_based":
		return "sistem custom (project based)"
	default:
		return "sistem"
	}
}

func bulletList(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	b := new(strings.Builder)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	return strings.TrimRight(b.String(), "\n")
}

func indentList(items []string) string {
	if len(items) == 0 {
		return "   - (tidak ada kebutuhan spesifik)"
	}
	b := new(strings.Builder)
	for i, it := range items {
		fmt.Fprintf(b, "   %d. %s\n", i+1, it)
	}
	return strings.TrimRight(b.String(), "\n")
}
