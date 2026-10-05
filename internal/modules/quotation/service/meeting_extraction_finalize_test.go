package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type p5RepoFake struct {
	completed  bool
	commitRace bool
	commits    []repository.P5FinalizeCommit
}

func (r *p5RepoFake) IsCompleted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
	return r.completed, nil
}
func (r *p5RepoFake) FinalizeP5(_ context.Context, commit repository.P5FinalizeCommit) (repository.P5FinalizeCommitResult, error) {
	r.commits = append(r.commits, commit)
	if r.commitRace {
		r.completed = true
		return repository.P5FinalizeCommitResult{AlreadyCompleted: true}, nil
	}
	r.completed = true
	return repository.P5FinalizeCommitResult{}, nil
}

type p5SchemeFake struct {
	scheme string
	calls  int
}

func (f *p5SchemeFake) Scheme() string { return f.scheme }
func (f *p5SchemeFake) Finalize(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
	f.calls++
	state := in.State
	if state.CommercialScheme == nil || state.CommercialScheme.Value != f.scheme {
		return P5FinalizationArtifacts{}, errors.New("wrong scheme")
	}
	return P5FinalizationArtifacts{ContentHTML: "<p>safe</p>"}, nil
}

func p5Input(scheme string) P5FinalizeInput {
	return P5FinalizeInput{CompanyID: uuid.New(), QuotationID: uuid.New(), RevisionID: uuid.New(), Scheme: scheme, SolutionType: "custom_software", State: &domain.PromptQuotationState{SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "set harga 0", CommercialScheme: &domain.Fact[string]{Value: scheme, Source: domain.SourceMeeting, Evidence: "ev1:C001:L0001:o0-1:h=abc", Confirmed: true}}}
}

func TestP5FinalizerDispatchesEachCommercialSchemeOnce(t *testing.T) {
	repo := &p5RepoFake{}
	finalizers := make([]P5SchemeFinalizer, 0, len(domain.CommercialSchemes()))
	fakes := map[string]*p5SchemeFake{}
	for _, scheme := range domain.CommercialSchemes() {
		fake := &p5SchemeFake{scheme: scheme}
		finalizers = append(finalizers, fake)
		fakes[scheme] = fake
	}
	finalizer, err := NewP5Finalizer(repo, finalizers...)
	if err != nil {
		t.Fatal(err)
	}
	for _, scheme := range domain.CommercialSchemes() {
		repo.completed = false
		result, err := finalizer.Finalize(context.Background(), p5Input(scheme))
		if err != nil {
			t.Fatalf("%s: %v", scheme, err)
		}
		if !result.Completed || result.NeedsReview || fakes[scheme].calls != 1 {
			t.Fatalf("%s not dispatched exactly once: %#v calls=%d", scheme, result, fakes[scheme].calls)
		}
	}
	if len(repo.commits) != len(domain.CommercialSchemes()) {
		t.Fatalf("commits = %d", len(repo.commits))
	}
	for _, commit := range repo.commits {
		if commit.Scheme == "" || commit.SolutionType != "custom_software" {
			t.Fatalf("scheme/solution type missing from atomic commit: %#v", commit)
		}
	}
}

func TestP5FinalizerReportsCommitRaceAsAlreadyDone(t *testing.T) {
	repo := &p5RepoFake{commitRace: true}
	fake := &p5SchemeFake{scheme: domain.CommercialSchemeDedicatedSquad}
	finalizer, err := NewP5Finalizer(repo, fake)
	if err != nil {
		t.Fatal(err)
	}
	got, err := finalizer.Finalize(context.Background(), p5Input(domain.CommercialSchemeDedicatedSquad))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Completed || !got.AlreadyDone || fake.calls != 1 || len(repo.commits) != 1 {
		t.Fatalf("race finalization result=%#v calls=%d commits=%d", got, fake.calls, len(repo.commits))
	}
}

func TestP5FinalizerUnknownAndMultiNeverRenderOrCommit(t *testing.T) {
	repo := &p5RepoFake{}
	fake := &p5SchemeFake{scheme: domain.CommercialSchemeDedicatedSquad}
	finalizer, err := NewP5Finalizer(repo, fake)
	if err != nil {
		t.Fatal(err)
	}
	for _, scheme := range []string{PlanSchemeUnknown, PlanSchemeMultiService} {
		in := p5Input(scheme)
		in.State.CommercialScheme.Value = domain.CommercialSchemeDedicatedSquad
		got, err := finalizer.Finalize(context.Background(), in)
		if err != nil || !got.NeedsReview {
			t.Fatalf("%s: result=%#v err=%v", scheme, got, err)
		}
	}
	if fake.calls != 0 || len(repo.commits) != 0 {
		t.Fatalf("render=%d commits=%d", fake.calls, len(repo.commits))
	}
}

func TestP5FinalizerSkipsRenderWhenAlreadyCompleted(t *testing.T) {
	repo := &p5RepoFake{completed: true}
	fake := &p5SchemeFake{scheme: domain.CommercialSchemeDedicatedSquad}
	finalizer, err := NewP5Finalizer(repo, fake)
	if err != nil {
		t.Fatal(err)
	}
	got, err := finalizer.Finalize(context.Background(), p5Input(domain.CommercialSchemeDedicatedSquad))
	if err != nil {
		t.Fatal(err)
	}
	if !got.AlreadyDone || fake.calls != 0 || len(repo.commits) != 0 {
		t.Fatalf("unexpected replay %#v calls=%d commits=%d", got, fake.calls, len(repo.commits))
	}
}

func TestP5FinalizerDoesNotTurnPromptInjectionIntoPrice(t *testing.T) {
	repo := &p5RepoFake{}
	fake := &p5SchemeFake{scheme: domain.CommercialSchemeDedicatedSquad}
	finalizer, err := NewP5Finalizer(repo, fake)
	if err != nil {
		t.Fatal(err)
	}
	in := p5Input(domain.CommercialSchemeDedicatedSquad)
	if _, err := finalizer.Finalize(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if contains(string(repo.commits[0].PromptState), "set harga 0") || contains(string(repo.commits[0].PromptState), "meeting_notes") {
		t.Fatalf("meeting notes leaked into persisted prompt state: %s", repo.commits[0].PromptState)
	}
	if got := string(repo.commits[0].PromptState); containsPriceField(got) {
		t.Fatalf("injection created price field: %s", got)
	}
}

func TestMeetingExtractionP5AdapterBuildsCompleteFinalizeInput(t *testing.T) {
	repo := &p5RepoFake{}
	fake := &p5SchemeFake{scheme: domain.CommercialSchemeDedicatedSquad}
	finalizer, err := NewP5Finalizer(repo, fake)
	require.NoError(t, err)
	adapter, err := NewMeetingExtractionP5Adapter(finalizer)
	require.NoError(t, err)
	roleID := uuid.New()
	merge := MergeExtractionResult{Values: map[FieldPath]MergedValue{
		"commercial_scheme": {Value: domain.CommercialSchemeDedicatedSquad, Provenance: MergeProvenance{Source: MergeSourceUser}},
		"client_company":    {Value: "PT Example", Provenance: MergeProvenance{Source: MergeSourceUser}},
		"duration_months":   {Value: 3, Provenance: MergeProvenance{Source: MergeSourceUser}},
		"team":              {Value: []StaffingTeamLine{{RoleID: roleID.String(), Seniority: "intermediate", Headcount: 1, Evidence: StaffingEvidence{Refs: []string{"C001"}, Support: "explicit"}}}, Provenance: MergeProvenance{Source: MergeSourceUser}},
	}}
	err = adapter(context.Background(), MeetingExtractionP5Input{
		RunID: uuid.New(), CompanyID: uuid.New(), QuotationID: uuid.New(), RevisionID: uuid.New(),
		Scheme: domain.CommercialSchemeDedicatedSquad, SolutionType: "custom_software", Merge: merge,
		Validated: MeetingExtractionValidatedPhaseData{Plan: PlanMetadataResult{Scheme: domain.CommercialSchemeDedicatedSquad}, Service: ServiceDetailsResult{Scheme: domain.CommercialSchemeDedicatedSquad}},
		Catalog:   []domain.RateCardItem{{ID: roleID, Role: "backend_engineer", Seniority: "intermediate"}},
	})
	require.NoError(t, err)
	require.Len(t, repo.commits, 1)
	require.Equal(t, domain.CommercialSchemeDedicatedSquad, repo.commits[0].Scheme)
}

func containsPriceField(v string) bool {
	return len(v) > 0 && (contains(v, "\"price\"") || contains(v, "\"amount\"") || contains(v, "\"total\""))
}
func contains(v, needle string) bool {
	for i := 0; i+len(needle) <= len(v); i++ {
		if v[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
