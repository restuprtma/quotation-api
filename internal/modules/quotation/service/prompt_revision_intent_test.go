package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type revisionIntentProviderStub struct {
	text   string
	err    error
	prompt string
}

func (s *revisionIntentProviderStub) Generate(_ context.Context, request GenerateRequest) (GenerateResult, error) {
	s.prompt = request.Prompt
	if s.err != nil {
		return GenerateResult{}, s.err
	}
	return GenerateResult{Text: s.text}, nil
}

type revisionIntentProviderSourceStub struct{ provider AIProvider }

func (s revisionIntentProviderSourceStub) ProviderForCompany(context.Context, uuid.UUID) (AIProvider, string, string, error) {
	return s.provider, "test", "test", nil
}

func TestPromptRevisionIntentService_ClassifiesSpecifiedInstructions(t *testing.T) {
	state := revisionIntentState()
	cases := []struct {
		name        string
		instruction string
		target      string
		output      string
		intent      string
		op          string
	}{
		{"duration", "Ubah pengerjaan menjadi empat bulan.", "", `{"schema_version":"prompt_revision_v1","intent":"commercial","target_section":"commercial","commercial_patch":[{"type":"set_duration_months","duration_months":4}]}`, "commercial", "set_duration_months"},
		{"maintenance", "Tambahkan maintenance tiga bulan.", "", `{"schema_version":"prompt_revision_v1","intent":"maintenance","target_section":"commercial","commercial_patch":[{"type":"maintenance_clarification"}]}`, "maintenance", "maintenance_clarification"},
		{"discount", "Berikan diskon lima persen.", "", `{"schema_version":"prompt_revision_v1","intent":"commercial","target_section":"commercial","commercial_patch":[{"type":"set_discount_bps","discount_bps":500}]}`, "commercial", "set_discount_bps"},
		{"scope", "Ringkas bagian ruang lingkup.", "scope", `{"schema_version":"prompt_revision_v1","intent":"narrative","target_section":"scope","replacement_text":"Inventory, stok masuk-keluar, approval, dan laporan."}`, "narrative", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &revisionIntentProviderStub{text: tc.output}
			svc := NewPromptRevisionIntentService(revisionIntentProviderSourceStub{provider: provider})
			intent, err := svc.Classify(context.Background(), uuid.New(), state, tc.instruction, tc.target, "Manual target text")
			require.NoError(t, err)
			require.Equal(t, tc.intent, intent.Intent)
			if tc.op != "" {
				require.Len(t, intent.CommercialPatch, 1)
				require.Equal(t, tc.op, intent.CommercialPatch[0].Type)
			}
			require.Contains(t, provider.prompt, "Manual target text")
			require.NotContains(t, provider.prompt, "TotalInvestment")
		})
	}
}

func TestPromptRevisionIntentService_RejectsUnsafeAndInvalidModelOutput(t *testing.T) {
	state := revisionIntentState()
	cases := []string{
		`{"schema_version":"prompt_revision_v1","intent":"narrative","target_section":"commercial","replacement_text":"<script>alert(1)</script>"}`,
		`{"schema_version":"prompt_revision_v1","intent":"narrative","target_section":"unknown","replacement_text":"text"}`,
		`Here is the JSON: {"schema_version":"prompt_revision_v1"}`,
		`{"schema_version":"prompt_revision_v1","intent":"narrative","target_section":"scope","replacement_text":"text","total":100}`,
	}
	for _, output := range cases {
		provider := &revisionIntentProviderStub{text: output}
		svc := NewPromptRevisionIntentService(revisionIntentProviderSourceStub{provider: provider})
		_, err := svc.Classify(context.Background(), uuid.New(), state, "Ringkas scope", "scope", "Manual scope")
		require.ErrorIs(t, err, ErrPromptRevisionInvalidIntent)
	}

	provider := &revisionIntentProviderStub{text: `{}`}
	svc := NewPromptRevisionIntentService(revisionIntentProviderSourceStub{provider: provider})
	_, err := svc.Classify(context.Background(), uuid.New(), state, " ", "scope", "Manual scope")
	require.ErrorIs(t, err, ErrPromptRevisionInvalidIntent)
}

func revisionIntentState() domain.PromptQuotationState {
	return domain.PromptQuotationState{SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "notes", Scope: []domain.Fact[string]{}, Team: []domain.PromptTeamLine{}, Issues: []domain.DraftIssue{}, Readiness: domain.ReadinessIncomplete}
}

func TestBuildPromptRevisionIntentPrompt_LimitsContextToCurrentStateAndRequestedSection(t *testing.T) {
	prompt, err := buildPromptRevisionIntentPrompt(revisionIntentState(), "Ringkas scope", "scope", "BA edited scope")
	require.NoError(t, err)
	require.Contains(t, prompt, "BA edited scope")
	require.NotContains(t, prompt, "<section")
	require.False(t, strings.Contains(prompt, "TotalInvestment"))
}
