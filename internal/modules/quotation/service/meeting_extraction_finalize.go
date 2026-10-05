package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

// ErrP5Finalization is deliberately safe to persist or expose through the
// extraction state machine.  It never contains an adapter, renderer, or
// provider error (those may include unsafe input).
var ErrP5Finalization = errors.New("p5 finalization failed")

// P5FinalizationArtifacts are the already-sanitized, deterministic artefacts
// produced by one existing scheme adapter/calculator/renderer.  P5 itself
// owns no pricing logic: an adapter must derive every commercial value from
// the immutable snapshots and backend catalog inputs it was given.
type P5FinalizationArtifacts = repository.P5FinalizationArtifacts

// P5SchemeFinalizer is the small common shape around the eight existing,
// otherwise deliberately different prompt adapters.  It lets P5 dispatch
// without flattening their typed calculator contracts into an unsafe map.
// Implementations must return incomplete/needs-input to their caller before
// this method when required commercial inputs are absent.
type P5SchemeFinalizer interface {
	Scheme() string
	Finalize(context.Context, P5AdapterInput) (P5FinalizationArtifacts, error)
}

// P5AdapterInput contains only the immutable draft snapshots and merged
// state needed by the existing typed adapter.  It deliberately contains no
// provider response or mutable live rate-card data.
type P5AdapterInput struct {
	State         *domain.PromptQuotationState
	Metadata      QuotationMetadata
	QuotationDate time.Time
	LegalHTML     string
	CompanyID     uuid.UUID
	RateCardID    string
	Catalog       []domain.RateCardItem
}

type p5DedicatedFinalizer struct {
	calculator promptDedicatedCalculator
	renderer   DedicatedSquadHTMLRenderer
}

func NewP5DedicatedFinalizer(calculator promptDedicatedCalculator, renderer DedicatedSquadHTMLRenderer) P5SchemeFinalizer {
	if renderer == nil {
		renderer = NewDedicatedSquadHTMLRenderer()
	}
	return &p5DedicatedFinalizer{calculator: calculator, renderer: renderer}
}
func (*p5DedicatedFinalizer) Scheme() string { return domain.CommercialSchemeDedicatedSquad }
func (f *p5DedicatedFinalizer) Finalize(ctx context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
	if f == nil || f.calculator == nil || in.CompanyID == uuid.Nil || strings.TrimSpace(in.RateCardID) == "" {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	anchor := in.QuotationDate
	input, issues := BuildDedicatedInput(in.State, in.Catalog, &anchor)
	if hasBlockingIssue(issues) {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	profile, err := ResolveDedicatedSquadProfile(input)
	if err != nil {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	calculation, err := f.calculator.CalculateWithProfile(ctx, in.CompanyID.String(), in.RateCardID, input, profile)
	if err != nil {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	rules, err := EvaluateDedicatedSquadRulesWithProfile(input, profile, calculation.DevelopmentSubtotal)
	if err != nil {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	modules := promptModules(in.State)
	timeline, err := GenerateTimeline(input, rules, modules)
	if err != nil {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	html, err := f.renderer.Render(DedicatedSquadDocumentInput{Metadata: in.Metadata, Calculation: calculation, Profile: profile, Rules: rules, Timeline: timeline})
	return p5Artifacts(html, profile, calculation, modules, timeline, err)
}

type p5AdapterFunc struct {
	scheme string
	fn     func(context.Context, P5AdapterInput) (P5FinalizationArtifacts, error)
}

func (f p5AdapterFunc) Scheme() string { return f.scheme }
func (f p5AdapterFunc) Finalize(ctx context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
	return f.fn(ctx, in)
}

// NewP5ExistingAdapterFinalizers adapts the seven typed specialized prompt
// adapters. Dedicated Squad keeps its existing calculator contract and is
// supplied separately by the worker composition root.  The wrappers do not
// add any pricing policy; Calculate remains the sole source of totals.
func NewP5ExistingAdapterFinalizers(registry *PromptServiceAdapterRegistry) []P5SchemeFinalizer {
	if registry == nil {
		registry = NewPromptServiceAdapterRegistry(nil, nil, nil, nil)
	}
	return []P5SchemeFinalizer{
		p5AdapterFunc{domain.CommercialSchemeSubscriptionSaaS, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.SubscriptionSaaS().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.SubscriptionSaaS().Render(in.Metadata, PromptSubscriptionSaaSSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, input.FeatureGroups, nil, err)
		}},
		p5AdapterFunc{domain.CommercialSchemeWhiteLabel, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.WhiteLabel().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.WhiteLabel().Render(in.Metadata, PromptWhiteLabelSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, input.ModuleGroups, input.TimelineRows, err)
		}},
		p5AdapterFunc{domain.CommercialSchemeStandaloneMaintenance, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.StandaloneMaintenance().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.StandaloneMaintenance().Render(in.Metadata, PromptStandaloneMaintenanceSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, nil, nil, err)
		}},
		p5AdapterFunc{domain.CommercialSchemeCustomDevelopment, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.CustomDevelopment().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.CustomDevelopment().Render(in.Metadata, PromptCustomDevelopmentSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, input.ModuleGroups, input.TimelineRows, err)
		}},
		p5AdapterFunc{domain.CommercialSchemeKioskQueueSystem, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.KioskQueue().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.KioskQueue().Render(in.Metadata, PromptKioskQueueSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, input.GovernmentScopePhases, input.PrivateTimelineRows, err)
		}},
		p5AdapterFunc{domain.CommercialSchemeTalentOutsourcing, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.TalentOutsourcing().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.TalentOutsourcing().Render(in.Metadata, PromptTalentOutsourcingSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, input.ScopeBlocks, nil, err)
		}},
		p5AdapterFunc{domain.CommercialSchemeCloudInfrastructure, func(_ context.Context, in P5AdapterInput) (P5FinalizationArtifacts, error) {
			input, calc, profile, issues, err := registry.CloudInfrastructure().Calculate(in.State, in.QuotationDate)
			if err != nil || hasBlockingIssue(issues) {
				return P5FinalizationArtifacts{}, ErrP5Finalization
			}
			html, err := registry.CloudInfrastructure().Render(in.Metadata, PromptCloudInfrastructureSnapshot{Input: input, Calculation: calc, Profile: profile}, in.LegalHTML)
			return p5Artifacts(html, profile, calc, input.ScopeRows, buildCloudTimelineSnapshot(input.ScopeRows), err)
		}},
	}
}

func p5Artifacts(html string, profile, calculation, module, timeline any, err error) (P5FinalizationArtifacts, error) {
	if err != nil {
		return P5FinalizationArtifacts{}, err
	}
	sanitized, err := NewHTMLSanitizer().Sanitize(html)
	if err != nil || strings.TrimSpace(sanitized) == "" {
		return P5FinalizationArtifacts{}, ErrP5Finalization
	}
	profileJSON, _ := json.Marshal(profile)
	calculationJSON, _ := json.Marshal(calculation)
	moduleJSON, _ := json.Marshal(module)
	timelineJSON, _ := json.Marshal(timeline)
	return P5FinalizationArtifacts{ContentHTML: sanitized, ResolvedProfileSnapshot: profileJSON, CalculationSnapshot: calculationJSON, ModuleSnapshot: moduleJSON, TimelineSnapshot: timelineJSON}, nil
}

// P5FinalizationRepository is the transaction boundary.  Its implementation
// must lock the current draft revision, return AlreadyCompleted before a
// second render is attempted, and atomically update the quotation scheme,
// solution type, prompt state, artefacts, and completed status.  Keeping this
// interface here makes the finalizer testable without a PostgreSQL worker.
type P5FinalizationRepository interface {
	IsCompleted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
	FinalizeP5(context.Context, repository.P5FinalizeCommit) (repository.P5FinalizeCommitResult, error)
}

type P5FinalizeInput struct {
	CompanyID     uuid.UUID
	QuotationID   uuid.UUID
	RevisionID    uuid.UUID
	Scheme        string
	SolutionType  string
	State         *domain.PromptQuotationState
	Metadata      QuotationMetadata
	QuotationDate time.Time
	LegalHTML     string
	RateCardID    string
	Catalog       []domain.RateCardItem
}

type P5FinalizeResult struct {
	Completed   bool
	AlreadyDone bool
	NeedsReview bool
}

// P5Finalizer is deterministic P5 dispatch.  unknown/multi_service are
// terminal review outcomes: they never mutate the temporary dedicated scheme
// and never invoke a calculator or renderer.
type P5Finalizer struct {
	repository P5FinalizationRepository
	byScheme   map[string]P5SchemeFinalizer
}

func NewP5Finalizer(repository P5FinalizationRepository, finalizers ...P5SchemeFinalizer) (*P5Finalizer, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrP5Finalization)
	}
	byScheme := make(map[string]P5SchemeFinalizer, len(finalizers))
	for _, finalizer := range finalizers {
		if finalizer == nil || !domain.IsCommercialScheme(finalizer.Scheme()) {
			return nil, fmt.Errorf("%w: invalid scheme finalizer", ErrP5Finalization)
		}
		if _, duplicate := byScheme[finalizer.Scheme()]; duplicate {
			return nil, fmt.Errorf("%w: duplicate scheme finalizer", ErrP5Finalization)
		}
		byScheme[finalizer.Scheme()] = finalizer
	}
	return &P5Finalizer{repository: repository, byScheme: byScheme}, nil
}

func (f *P5Finalizer) Finalize(ctx context.Context, input P5FinalizeInput) (P5FinalizeResult, error) {
	if f == nil || f.repository == nil || input.CompanyID == uuid.Nil || input.QuotationID == uuid.Nil || input.RevisionID == uuid.Nil || input.State == nil {
		return P5FinalizeResult{}, fmt.Errorf("%w: invalid input", ErrP5Finalization)
	}
	scheme := strings.TrimSpace(input.Scheme)
	if scheme == PlanSchemeUnknown || scheme == PlanSchemeMultiService {
		return P5FinalizeResult{NeedsReview: true}, nil
	}
	if !domain.IsCommercialScheme(scheme) {
		return P5FinalizeResult{}, fmt.Errorf("%w: unsupported scheme", ErrP5Finalization)
	}
	finalizer := f.byScheme[scheme]
	if finalizer == nil {
		return P5FinalizeResult{}, fmt.Errorf("%w: scheme finalizer unavailable", ErrP5Finalization)
	}
	completed, err := f.repository.IsCompleted(ctx, input.CompanyID, input.QuotationID, input.RevisionID)
	if err != nil {
		return P5FinalizeResult{}, fmt.Errorf("%w: completion lookup", ErrP5Finalization)
	}
	if completed {
		return P5FinalizeResult{Completed: true, AlreadyDone: true}, nil
	}

	// Never let the finalizer alter the selected scheme or fabricate a value.
	// The merged state is the sole input to the typed existing adapter.
	state := cloneP5State(input.State)
	if state.CommercialScheme == nil || state.CommercialScheme.Value != scheme {
		return P5FinalizeResult{}, fmt.Errorf("%w: state scheme mismatch", ErrP5Finalization)
	}
	artifacts, err := finalizer.Finalize(ctx, P5AdapterInput{State: state, Metadata: input.Metadata, QuotationDate: input.QuotationDate, LegalHTML: input.LegalHTML, CompanyID: input.CompanyID, RateCardID: input.RateCardID, Catalog: input.Catalog})
	if err != nil || strings.TrimSpace(artifacts.ContentHTML) == "" {
		return P5FinalizeResult{}, fmt.Errorf("%w: scheme finalization", ErrP5Finalization)
	}
	stateJSON, err := MarshalPromptQuotationStateForPersistence(state)
	if err != nil {
		return P5FinalizeResult{}, fmt.Errorf("%w: state encoding", ErrP5Finalization)
	}
	commit, err := f.repository.FinalizeP5(ctx, repository.P5FinalizeCommit{
		CompanyID: input.CompanyID, QuotationID: input.QuotationID, RevisionID: input.RevisionID,
		Scheme: scheme, SolutionType: strings.TrimSpace(input.SolutionType), PromptState: stateJSON, Artifacts: artifacts,
	})
	if err != nil {
		return P5FinalizeResult{}, fmt.Errorf("%w: transaction", ErrP5Finalization)
	}
	return P5FinalizeResult{Completed: true, AlreadyDone: commit.AlreadyCompleted}, nil
}

func cloneP5State(state *domain.PromptQuotationState) *domain.PromptQuotationState {
	encoded, _ := json.Marshal(state)
	var clone domain.PromptQuotationState
	_ = json.Unmarshal(encoded, &clone)
	return &clone
}
