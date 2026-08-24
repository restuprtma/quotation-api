package rnd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// BlueprintRunnerConfig configures the execution environment of the BlueprintRunner.
type BlueprintRunnerConfig struct {
	ArtifactRoot string
	OutputDir    string
	RawDir       string
	Provider     Provider
	Model        string
	Runs         int
	Clock        func() time.Time
}

// BlueprintRunner orchestrates the 7-stage sequential generation and validation pipeline for blueprints.
type BlueprintRunner struct {
	config          BlueprintRunnerConfig
	schemaValidator *BlueprintSchemaValidator
	crossValidator  *BlueprintCrossValidator
}

// NewBlueprintRunner validates the configuration and creates a new BlueprintRunner.
func NewBlueprintRunner(cfg BlueprintRunnerConfig) (*BlueprintRunner, error) {
	if strings.TrimSpace(cfg.ArtifactRoot) == "" {
		return nil, errors.New("artifact root is required")
	}
	if strings.TrimSpace(cfg.OutputDir) == "" {
		return nil, errors.New("output directory is required")
	}
	if strings.TrimSpace(cfg.RawDir) == "" {
		return nil, errors.New("raw directory is required")
	}
	if cfg.Provider == nil {
		return nil, errors.New("provider is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("model is required")
	}
	if cfg.Runs < 1 || cfg.Runs > 10 {
		return nil, errors.New("runs must be between 1 and 10")
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}

	return &BlueprintRunner{
		config:          cfg,
		schemaValidator: NewBlueprintSchemaValidator(),
		crossValidator:  NewBlueprintCrossValidator(),
	}, nil
}

func sanitizeFixtureName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var sb strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	res := sb.String()
	if res == "" {
		return "blueprint-run"
	}
	return res
}

func (r *BlueprintRunner) writeReport(path string, report *BlueprintBatchReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}
	return nil
}

func (r *BlueprintRunner) writeRaw(safeFixture string, stage BlueprintStage, raw string) error {
	if strings.TrimSpace(r.config.RawDir) == "" || raw == "" {
		return nil
	}
	filename := fmt.Sprintf("%s-%s.raw", safeFixture, stage)
	path := filepath.Join(r.config.RawDir, filename)
	return os.WriteFile(path, []byte(raw), 0o600)
}

// Run executes the 7-stage pipeline with failure isolation against the specified fixture.
func (r *BlueprintRunner) Run(ctx context.Context, fixturePath string) (*BlueprintBatchReport, error) {
	safeFixture := sanitizeFixtureName(fixturePath)
	report := &BlueprintBatchReport{
		SchemaVersion: BlueprintSchemaVersion,
		Fixture:       safeFixture,
		Provider:      r.config.Provider.Name(),
		Model:         r.config.Model,
		PromptVersion: "1.0",
		Status:        "pending",
		StartedAt:     r.config.Clock(),
		Stages: []BlueprintStageResult{
			{Stage: StageContextAnalysis, Status: StatusPending},
			{Stage: StageJourney, Status: StatusPending},
			{Stage: StageArchitecture, Status: StatusPending},
			{Stage: StageStateMachine, Status: StatusPending},
			{Stage: StageWorkflow, Status: StatusPending},
			{Stage: StageWBS, Status: StatusPending},
			{Stage: StageCrossValidation, Status: StatusPending},
		},
		Bundle: &BlueprintBundle{
			SchemaVersion: BlueprintSchemaVersion,
		},
	}
	return r.Resume(ctx, fixturePath, report)
}

// Resume continues or retries a pipeline execution against an existing report and bundle state.
// It skips stages already completed or validated, and executes any failed, pending, or manual stages.
func (r *BlueprintRunner) Resume(ctx context.Context, fixturePath string, report *BlueprintBatchReport) (*BlueprintBatchReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if report == nil {
		return nil, errors.New("report is required for resume")
	}

	startedAt := r.config.Clock()
	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("read fixture file: %w", err)
	}

	var input BlueprintInput
	if err := json.Unmarshal(fixtureData, &input); err != nil {
		return nil, fmt.Errorf("parse fixture JSON: %w", err)
	}

	safeFixture := sanitizeFixtureName(fixturePath)
	if err := os.MkdirAll(r.config.OutputDir, 0o750); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	if err := os.MkdirAll(r.config.RawDir, 0o700); err != nil {
		return nil, fmt.Errorf("create raw directory: %w", err)
	}

	if report.Bundle == nil {
		report.Bundle = &BlueprintBundle{SchemaVersion: BlueprintSchemaVersion}
	}

	stages := []BlueprintStage{
		StageContextAnalysis,
		StageJourney,
		StageArchitecture,
		StageStateMachine,
		StageWorkflow,
		StageWBS,
		StageCrossValidation,
	}

	// Ensure report has 7 stages
	if len(report.Stages) != len(stages) {
		existingStages := make(map[BlueprintStage]BlueprintStageResult)
		for _, st := range report.Stages {
			existingStages[st.Stage] = st
		}
		newStages := make([]BlueprintStageResult, len(stages))
		for i, stName := range stages {
			if existing, ok := existingStages[stName]; ok {
				newStages[i] = existing
			} else {
				newStages[i] = BlueprintStageResult{Stage: stName, Status: StatusPending}
			}
		}
		report.Stages = newStages
	}

	var totalBlockingErrors int
	var totalWarnings int
	var pipelineFailed bool
	var failureErr error

	for i, stage := range stages {
		if err := ctx.Err(); err != nil {
			report.Stages[i].Status = StatusFailed
			report.Stages[i].Error = err.Error()
			pipelineFailed = true
			failureErr = err
			break
		}

		currStatus := report.Stages[i].Status
		if currStatus == StatusCompleted {
			// Stage already completed successfully in previous run. Re-validate to collect issues.
			issues := r.validateStage(stage, report.Bundle, &input)
			report.Stages[i].Issues = issues
			for _, iss := range issues {
				if iss.Blocking || iss.Severity == SeverityBlockingError {
					totalBlockingErrors++
				} else if iss.Severity == SeverityWarning {
					totalWarnings++
				}
			}
			continue
		}

		if currStatus == StatusValidated {
			// Stage was manually completed/edited by BA and set to validated. Validate schema.
			issues := r.validateStage(stage, report.Bundle, &input)
			report.Stages[i].Issues = issues
			hasBlocking := false
			for _, iss := range issues {
				if iss.Blocking || iss.Severity == SeverityBlockingError {
					hasBlocking = true
					totalBlockingErrors++
				} else if iss.Severity == SeverityWarning {
					totalWarnings++
				}
			}
			if hasBlocking {
				report.Stages[i].Status = StatusFailed
				report.Stages[i].Error = "manual payload validation failed"
				pipelineFailed = true
				break
			}
			// Retain StatusValidated
			continue
		}

		// Stage needs generation / retry
		stageStart := r.config.Clock()
		report.Stages[i].StartedAt = stageStart
		report.Stages[i].Status = StatusGenerating
		report.Stages[i].RetryCount++

		var stageIssues []ValidationIssue
		var genErr error

		if stage == StageCrossValidation {
			// Stage 7 is deterministic cross-validation & bundle schema integrity check
			stageIssues := r.validateStage(stage, report.Bundle, &input)
			report.Stages[i].Issues = stageIssues
			hasBlocking := false
			for _, iss := range stageIssues {
				if iss.Blocking || iss.Severity == SeverityBlockingError {
					hasBlocking = true
					totalBlockingErrors++
				} else if iss.Severity == SeverityWarning {
					totalWarnings++
				}
			}
			stageDuration := r.config.Clock().Sub(stageStart)
			report.Stages[i].DurationMS = stageDuration.Milliseconds()
			if hasBlocking {
				report.Stages[i].Status = StatusFailed
				pipelineFailed = true
				break
			}
			report.Stages[i].Status = StatusCompleted
			continue
		}

		systemPrompt, instructionPrompt := r.getPrompt(stage)
		resp, pErr := r.config.Provider.Generate(ctx, ProviderRequest{
			Stage:       Stage(stage),
			Model:       r.config.Model,
			System:      systemPrompt,
			Instruction: instructionPrompt,
			Payload: map[string]interface{}{
				"stage":  stage,
				"input":  input,
				"bundle": report.Bundle,
			},
		})

		if pErr != nil {
			genErr = pErr
		} else {
			report.Stages[i].PromptTokens = resp.PromptTokens
			report.Stages[i].CandidateTokens = resp.CandidateTokens
			report.Stages[i].TotalTokens = resp.TotalTokens
			if resp.Raw != "" {
				_ = r.writeRaw(safeFixture, stage, resp.Raw)
				genErr = r.applyStagePayload(stage, resp.Raw, report.Bundle)
			} else {
				genErr = r.synthesizeMockStage(stage, &input, report.Bundle)
			}
		}

		stageDuration := r.config.Clock().Sub(stageStart)
		report.Stages[i].DurationMS = stageDuration.Milliseconds()

		if genErr != nil {
			report.Stages[i].Status = StatusFailed
			report.Stages[i].Error = genErr.Error()
			pipelineFailed = true
			failureErr = genErr
			break
		}

		// Validate Stage Schemas
		stageIssues = r.validateStage(stage, report.Bundle, &input)
		report.Stages[i].Issues = stageIssues

		hasBlocking := false
		for _, iss := range stageIssues {
			if iss.Blocking || iss.Severity == SeverityBlockingError {
				hasBlocking = true
				totalBlockingErrors++
			} else if iss.Severity == SeverityWarning {
				totalWarnings++
			}
		}

		if hasBlocking {
			report.Stages[i].Status = StatusFailed
			pipelineFailed = true
			break
		}

		report.Stages[i].Status = StatusCompleted
	}

	completedCount := 0
	failedCount := 0
	for _, st := range report.Stages {
		if st.Status == StatusCompleted || st.Status == StatusValidated {
			completedCount++
		} else if st.Status == StatusFailed {
			failedCount++
		}
	}

	if pipelineFailed || failedCount > 0 || totalBlockingErrors > 0 {
		report.Status = "fail"
	} else {
		report.Status = "pass"
	}

	report.Summary = BlueprintReportSummary{
		TotalStages:        len(stages),
		CompletedStages:    completedCount,
		FailedStages:       failedCount,
		BlockingErrors:     totalBlockingErrors,
		WarningsCount:      totalWarnings,
		Passed:             report.Status == "pass",
		CanPublishPartial:  completedCount >= 1 && totalBlockingErrors == 0,
		CanPublishComplete: completedCount == len(stages) && totalBlockingErrors == 0,
	}

	report.CompletedAt = r.config.Clock()
	if report.StartedAt.IsZero() {
		report.StartedAt = startedAt
	}
	report.TotalLatencyMS = report.CompletedAt.Sub(report.StartedAt).Milliseconds()

	var totalPromptTokens, totalCandidateTokens, totalTokens int
	for _, st := range report.Stages {
		totalPromptTokens += st.PromptTokens
		totalCandidateTokens += st.CandidateTokens
		totalTokens += st.TotalTokens
	}
	report.TotalPromptTokens = totalPromptTokens
	report.TotalCandidateTokens = totalCandidateTokens
	report.TotalTokens = totalTokens
	report.EstimatedCostUSD = calculateEstimatedCost(r.config.Model, totalPromptTokens, totalCandidateTokens)

	// Persist report JSON
	reportFilePath := filepath.Join(r.config.OutputDir, fmt.Sprintf("%s-report.json", safeFixture))
	_ = r.writeReport(reportFilePath, report)

	if failureErr != nil && errors.Is(failureErr, context.Canceled) {
		return report, failureErr
	}

	return report, nil
}

func calculateEstimatedCost(model string, promptTokens, candidateTokens int) float64 {
	m := strings.ToLower(model)
	if strings.Contains(m, "flash-lite") {
		return (float64(promptTokens)*0.075 + float64(candidateTokens)*0.30) / 1000000.0
	}
	if strings.Contains(m, "flash") {
		return (float64(promptTokens)*0.15 + float64(candidateTokens)*0.60) / 1000000.0
	}
	return 0.0
}

func (r *BlueprintRunner) getPrompt(stage BlueprintStage) (system string, instruction string) {
	if strings.TrimSpace(r.config.ArtifactRoot) == "" {
		return "", ""
	}

	safetyPath := filepath.Join(r.config.ArtifactRoot, "safety-envelope.md")
	if data, err := os.ReadFile(safetyPath); err == nil {
		system = string(data)
	}

	var promptFile string
	switch stage {
	case StageContextAnalysis:
		promptFile = "context-prompt-v1.md"
	case StageJourney:
		promptFile = "journey-prompt-v1.md"
	case StageArchitecture:
		promptFile = "architecture-prompt-v1.md"
	case StageStateMachine:
		promptFile = "state-machine-prompt-v1.md"
	case StageWorkflow:
		promptFile = "workflow-prompt-v1.md"
	case StageWBS:
		promptFile = "wbs-prompt-v1.md"
	}

	if promptFile != "" {
		promptPath := filepath.Join(r.config.ArtifactRoot, promptFile)
		if data, err := os.ReadFile(promptPath); err == nil {
			instruction = string(data)
		}
	}
	return system, instruction
}

var (
	reKeyColon            = regexp.MustCompile(`"([a-zA-Z0-9_]+)":\s*"([a-zA-Z0-9_]+)":`)
	reEqualVal            = regexp.MustCompile(`:\s*"[^"]+"\s*=\s*"([^"]+)"`)
	reMissingLeadingQuote = regexp.MustCompile(`(?m)(^\s*)\.([a-zA-Z0-9_\-\.]+)"`)
	reTrailingComma       = regexp.MustCompile(`,\s*([}\]])`)
)

func sanitizeJSONSyntax(s string) string {
	s = reKeyColon.ReplaceAllStringFunc(s, func(m string) string {
		sub := reKeyColon.FindStringSubmatch(m)
		if len(sub) == 3 && sub[1] == sub[2] {
			return fmt.Sprintf(`"%s":`, sub[1])
		}
		return m
	})
	s = reEqualVal.ReplaceAllString(s, `: "$1"`)
	s = reMissingLeadingQuote.ReplaceAllString(s, `${1}"$2"`)
	s = reTrailingComma.ReplaceAllString(s, `$1`)
	return s
}

func extractCleanJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = strings.TrimSpace(raw[idx+1:])
		}
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}
	startObj := strings.Index(raw, "{")
	startArr := strings.Index(raw, "[")
	var candidate string
	if startObj != -1 && (startArr == -1 || startObj < startArr) {
		endObj := strings.LastIndex(raw, "}")
		if endObj > startObj {
			candidate = raw[startObj : endObj+1]
		}
	} else if startArr != -1 {
		endArr := strings.LastIndex(raw, "]")
		if endArr > startArr {
			candidate = raw[startArr : endArr+1]
		}
	}
	if candidate == "" {
		candidate = raw
	}
	return sanitizeJSONSyntax(candidate)
}

func (r *BlueprintRunner) validateStage(stage BlueprintStage, bundle *BlueprintBundle, input *BlueprintInput) []ValidationIssue {
	if bundle == nil {
		return nil
	}

	switch stage {
	case StageJourney:
		return r.schemaValidator.ValidateJourney(&bundle.Journey)
	case StageArchitecture:
		return r.schemaValidator.ValidateArchitecture(&bundle.Architecture)
	case StageStateMachine:
		return r.schemaValidator.ValidateStateMachine(&bundle.StateMachine)
	case StageWorkflow:
		return r.schemaValidator.ValidateWorkflows(bundle.Workflows)
	case StageWBS:
		return r.schemaValidator.ValidateWBS(&bundle.WBS)
	case StageCrossValidation:
		bundleIssues := r.schemaValidator.ValidateBundle(bundle)
		crossIssues := r.crossValidator.CrossValidate(bundle, input)
		return append(bundleIssues, crossIssues...)
	default:
		return nil
	}
}

func (r *BlueprintRunner) applyStagePayload(stage BlueprintStage, raw string, bundle *BlueprintBundle) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	clean := extractCleanJSON(raw)

	switch stage {
	case StageContextAnalysis:
		var ctxPayload struct {
			Title         string         `json:"title"`
			Summary       string         `json:"summary"`
			Assumptions   []string       `json:"assumptions"`
			OpenQuestions []OpenQuestion `json:"open_questions"`
		}
		if err := json.Unmarshal([]byte(clean), &ctxPayload); err == nil && (ctxPayload.Title != "" || ctxPayload.Summary != "") {
			bundle.Title = ctxPayload.Title
			bundle.Summary = ctxPayload.Summary
			bundle.Assumptions = ctxPayload.Assumptions
			bundle.OpenQuestions = ctxPayload.OpenQuestions
			return nil
		}
		var wrapped struct {
			ContextAnalysis *struct {
				Title         string         `json:"title"`
				Summary       string         `json:"summary"`
				Assumptions   []string       `json:"assumptions"`
				OpenQuestions []OpenQuestion `json:"open_questions"`
			} `json:"context_analysis"`
			Bundle *struct {
				Title         string         `json:"title"`
				Summary       string         `json:"summary"`
				Assumptions   []string       `json:"assumptions"`
				OpenQuestions []OpenQuestion `json:"open_questions"`
			} `json:"bundle"`
		}
		if errWrap := json.Unmarshal([]byte(clean), &wrapped); errWrap == nil {
			if wrapped.ContextAnalysis != nil {
				bundle.Title = wrapped.ContextAnalysis.Title
				bundle.Summary = wrapped.ContextAnalysis.Summary
				bundle.Assumptions = wrapped.ContextAnalysis.Assumptions
				bundle.OpenQuestions = wrapped.ContextAnalysis.OpenQuestions
				return nil
			}
			if wrapped.Bundle != nil {
				bundle.Title = wrapped.Bundle.Title
				bundle.Summary = wrapped.Bundle.Summary
				bundle.Assumptions = wrapped.Bundle.Assumptions
				bundle.OpenQuestions = wrapped.Bundle.OpenQuestions
				return nil
			}
		}
		if err := json.Unmarshal([]byte(clean), &ctxPayload); err != nil {
			return fmt.Errorf("parse context analysis JSON: %w", err)
		}
		bundle.Title = ctxPayload.Title
		bundle.Summary = ctxPayload.Summary
		bundle.Assumptions = ctxPayload.Assumptions
		bundle.OpenQuestions = ctxPayload.OpenQuestions
		return nil

	case StageJourney:
		var journey JourneySection
		if err := json.Unmarshal([]byte(clean), &journey); err == nil && (len(journey.Phases) > 0 || len(journey.Actors) > 0 || len(journey.Activities) > 0) {
			bundle.Journey = journey
			return nil
		}
		var wrapped struct {
			Journey JourneySection `json:"journey"`
		}
		if err := json.Unmarshal([]byte(clean), &wrapped); err == nil && (len(wrapped.Journey.Phases) > 0 || len(wrapped.Journey.Actors) > 0 || len(wrapped.Journey.Activities) > 0) {
			bundle.Journey = wrapped.Journey
			return nil
		}
		var wrappedBundle struct {
			Bundle struct {
				Journey JourneySection `json:"journey"`
			} `json:"bundle"`
		}
		if err := json.Unmarshal([]byte(clean), &wrappedBundle); err == nil && (len(wrappedBundle.Bundle.Journey.Phases) > 0 || len(wrappedBundle.Bundle.Journey.Actors) > 0 || len(wrappedBundle.Bundle.Journey.Activities) > 0) {
			bundle.Journey = wrappedBundle.Bundle.Journey
			return nil
		}
		if err := json.Unmarshal([]byte(clean), &journey); err != nil {
			return fmt.Errorf("parse journey JSON: %w", err)
		}
		bundle.Journey = journey
		return nil

	case StageArchitecture:
		var arch ArchitectureSection
		if err := json.Unmarshal([]byte(clean), &arch); err == nil && (len(arch.Groups) > 0 || len(arch.Nodes) > 0 || len(arch.Edges) > 0) {
			bundle.Architecture = arch
			return nil
		}
		var wrapped struct {
			Architecture ArchitectureSection `json:"architecture"`
		}
		if err := json.Unmarshal([]byte(clean), &wrapped); err == nil && (len(wrapped.Architecture.Groups) > 0 || len(wrapped.Architecture.Nodes) > 0 || len(wrapped.Architecture.Edges) > 0) {
			bundle.Architecture = wrapped.Architecture
			return nil
		}
		var wrappedBundle struct {
			Bundle struct {
				Architecture ArchitectureSection `json:"architecture"`
			} `json:"bundle"`
		}
		if err := json.Unmarshal([]byte(clean), &wrappedBundle); err == nil && (len(wrappedBundle.Bundle.Architecture.Groups) > 0 || len(wrappedBundle.Bundle.Architecture.Nodes) > 0 || len(wrappedBundle.Bundle.Architecture.Edges) > 0) {
			bundle.Architecture = wrappedBundle.Bundle.Architecture
			return nil
		}
		if err := json.Unmarshal([]byte(clean), &arch); err != nil {
			return fmt.Errorf("parse architecture JSON: %w", err)
		}
		bundle.Architecture = arch
		return nil

	case StageStateMachine:
		var sm StateMachineSection
		if err := json.Unmarshal([]byte(clean), &sm); err == nil && (sm.Entity != "" || len(sm.States) > 0) {
			bundle.StateMachine = sm
			return nil
		}
		var wrapped struct {
			StateMachine StateMachineSection `json:"state_machine"`
		}
		if err := json.Unmarshal([]byte(clean), &wrapped); err == nil && (wrapped.StateMachine.Entity != "" || len(wrapped.StateMachine.States) > 0) {
			bundle.StateMachine = wrapped.StateMachine
			return nil
		}
		var wrappedBundle struct {
			Bundle struct {
				StateMachine StateMachineSection `json:"state_machine"`
			} `json:"bundle"`
		}
		if err := json.Unmarshal([]byte(clean), &wrappedBundle); err == nil && (wrappedBundle.Bundle.StateMachine.Entity != "" || len(wrappedBundle.Bundle.StateMachine.States) > 0) {
			bundle.StateMachine = wrappedBundle.Bundle.StateMachine
			return nil
		}
		if err := json.Unmarshal([]byte(clean), &sm); err != nil {
			return fmt.Errorf("parse state machine JSON: %w", err)
		}
		bundle.StateMachine = sm
		return nil

	case StageWorkflow:
		var workflows []Workflow
		if err := json.Unmarshal([]byte(clean), &workflows); err == nil && len(workflows) > 0 {
			bundle.Workflows = workflows
			return nil
		}
		var single Workflow
		if err := json.Unmarshal([]byte(clean), &single); err == nil && (single.ID != "" || single.Name != "" || len(single.Nodes) > 0) {
			bundle.Workflows = []Workflow{single}
			return nil
		}
		var wrapped struct {
			Workflows []Workflow `json:"workflows"`
		}
		if err := json.Unmarshal([]byte(clean), &wrapped); err == nil && len(wrapped.Workflows) > 0 {
			bundle.Workflows = wrapped.Workflows
			return nil
		}
		var wrappedBundle struct {
			Bundle struct {
				Workflows []Workflow `json:"workflows"`
			} `json:"bundle"`
		}
		if err := json.Unmarshal([]byte(clean), &wrappedBundle); err == nil && len(wrappedBundle.Bundle.Workflows) > 0 {
			bundle.Workflows = wrappedBundle.Bundle.Workflows
			return nil
		}
		if err := json.Unmarshal([]byte(clean), &workflows); err != nil {
			return fmt.Errorf("parse workflow JSON: %w", err)
		}
		bundle.Workflows = workflows
		return nil

	case StageWBS:
		var wbsPayload struct {
			Modules         []WBSModule      `json:"modules"`
			CrossReferences []CrossReference `json:"cross_references"`
		}
		if err := json.Unmarshal([]byte(clean), &wbsPayload); err == nil && len(wbsPayload.Modules) > 0 {
			bundle.WBS = WBSSection{Modules: wbsPayload.Modules}
			if len(wbsPayload.CrossReferences) > 0 {
				bundle.CrossReferences = wbsPayload.CrossReferences
			}
			return nil
		}
		var wrapped struct {
			WBS             WBSSection       `json:"wbs"`
			CrossReferences []CrossReference `json:"cross_references"`
		}
		if err := json.Unmarshal([]byte(clean), &wrapped); err == nil && len(wrapped.WBS.Modules) > 0 {
			bundle.WBS = wrapped.WBS
			if len(wrapped.CrossReferences) > 0 {
				bundle.CrossReferences = wrapped.CrossReferences
			}
			return nil
		}
		var wrappedBundle struct {
			Bundle struct {
				WBS             WBSSection       `json:"wbs"`
				CrossReferences []CrossReference `json:"cross_references"`
			} `json:"bundle"`
		}
		if err := json.Unmarshal([]byte(clean), &wrappedBundle); err == nil && len(wrappedBundle.Bundle.WBS.Modules) > 0 {
			bundle.WBS = wrappedBundle.Bundle.WBS
			if len(wrappedBundle.Bundle.CrossReferences) > 0 {
				bundle.CrossReferences = wrappedBundle.Bundle.CrossReferences
			}
			return nil
		}
		var wbs WBSSection
		if err := json.Unmarshal([]byte(clean), &wbs); err != nil {
			return fmt.Errorf("parse wbs JSON: %w", err)
		}
		bundle.WBS = wbs
		return nil

	case StageCrossValidation:
		return nil

	default:
		return fmt.Errorf("unsupported stage payload %q", stage)
	}
}

func (r *BlueprintRunner) synthesizeMockStage(stage BlueprintStage, input *BlueprintInput, bundle *BlueprintBundle) error {
	switch stage {
	case StageContextAnalysis:
		title := input.SourceContext.Subject
		if title == "" {
			title = "Core Banking Migration"
		}
		summary := input.SourceContext.Summary
		if summary == "" {
			summary = "High-scale transactional banking migration to cloud microservices."
		}
		bundle.Title = title
		bundle.Summary = summary
		bundle.Assumptions = []string{
			"Standard cloud hosting environment with 99.9% uptime SLA.",
			"Modern web and mobile client browser support.",
		}
		bundle.OpenQuestions = []OpenQuestion{}
		bundle.CrossReferences = []CrossReference{}
		return nil

	case StageJourney:
		phases := make([]JourneyPhase, 0, len(input.BusinessPhases))
		if len(input.BusinessPhases) > 0 {
			for i, p := range input.BusinessPhases {
				phases = append(phases, JourneyPhase{
					ID:    fmt.Sprintf("phase-%d", i+1),
					Label: p,
					Order: i + 1,
				})
			}
		} else {
			phases = append(phases,
				JourneyPhase{ID: "phase-1", Label: "Discovery", Order: 1},
				JourneyPhase{ID: "phase-2", Label: "Execution", Order: 2},
			)
		}

		actors := make([]JourneyActor, 0, len(input.Actors))
		if len(input.Actors) > 0 {
			for i, a := range input.Actors {
				actors = append(actors, JourneyActor{
					ID:    fmt.Sprintf("actor-%d", i+1),
					Label: a,
					Order: i + 1,
				})
			}
		} else {
			actors = append(actors,
				JourneyActor{ID: "actor-1", Label: "Customer", Order: 1},
				JourneyActor{ID: "actor-2", Label: "Admin", Order: 2},
			)
		}

		p1 := phases[0].ID
		a1 := actors[0].ID
		activities := []JourneyActivity{
			{
				ID:          "act-1",
				PhaseID:     p1,
				ActorID:     a1,
				Label:       "Submit Request",
				Description: "User submits request via system channel",
				Touchpoints: []string{"Web Portal", "Mobile App"},
				References:  []string{"architecture.node.node-web"},
			},
		}
		if len(phases) > 1 {
			activities = append(activities, JourneyActivity{
				ID:          "act-2",
				PhaseID:     phases[1].ID,
				ActorID:     a1,
				Label:       "Process Transaction",
				Description: "System processes and settles the transaction",
				Touchpoints: []string{"Web Portal"},
				References:  []string{"architecture.node.node-api"},
			})
		}

		bundle.Journey = JourneySection{
			Phases:     phases,
			Actors:     actors,
			Activities: activities,
		}
		return nil

	case StageArchitecture:
		groups := []ArchGroup{
			{ID: "grp-frontend", Label: "Frontend Channels", Order: 1},
			{ID: "grp-backend", Label: "Application Services", Order: 2},
			{ID: "grp-storage", Label: "Persistence Layer", Order: 3},
		}

		nodes := []ArchNode{
			{
				ID:          "node-web",
				GroupID:     "grp-frontend",
				Type:        "channel",
				Label:       "Web Portal",
				Description: "Web Application Interface",
				Layout:      LayoutCoordinate{X: 100, Y: 100},
			},
			{
				ID:          "node-api",
				GroupID:     "grp-backend",
				Type:        "service",
				Label:       "Core API Service",
				Description: "Microservice Application API",
				Layout:      LayoutCoordinate{X: 250, Y: 100},
			},
			{
				ID:          "node-db",
				GroupID:     "grp-storage",
				Type:        "database",
				Label:       "Primary Database",
				Description: "PostgreSQL Relational DB",
				Layout:      LayoutCoordinate{X: 400, Y: 100},
			},
		}

		edges := []ArchEdge{
			{
				ID:     "edge-1",
				Source: "node-web",
				Target: "node-api",
				Label:  "HTTPS/REST",
				Type:   "sync",
			},
			{
				ID:     "edge-2",
				Source: "node-api",
				Target: "node-db",
				Label:  "TCP/SQL",
				Type:   "data_flow",
			},
		}

		bundle.Architecture = ArchitectureSection{
			Groups: groups,
			Nodes:  nodes,
			Edges:  edges,
		}
		return nil

	case StageStateMachine:
		entity := "Transaction"
		if input.SourceContext.Subject != "" {
			entity = input.SourceContext.Subject
		}

		states := []SMState{
			{ID: "state-draft", Label: "Draft", Type: "normal", Layout: LayoutCoordinate{X: 50, Y: 50}},
			{ID: "state-processing", Label: "Processing", Type: "normal", Layout: LayoutCoordinate{X: 200, Y: 50}},
			{ID: "state-completed", Label: "Completed", Type: "terminal", Layout: LayoutCoordinate{X: 350, Y: 50}},
		}

		actorID := "actor-1"
		if len(bundle.Journey.Actors) > 0 {
			actorID = bundle.Journey.Actors[0].ID
		}

		transitions := []SMTransition{
			{
				ID:      "trans-1",
				Source:  "state-draft",
				Target:  "state-processing",
				Event:   "SUBMIT",
				Actor:   actorID,
				Actions: []string{"Submit Request"},
			},
			{
				ID:      "trans-2",
				Source:  "state-processing",
				Target:  "state-completed",
				Event:   "COMPLETE",
				Actor:   actorID,
				Actions: []string{"Process Transaction"},
			},
		}

		bundle.StateMachine = StateMachineSection{
			Entity:       entity,
			InitialState: "state-draft",
			States:       states,
			Transitions:  transitions,
		}
		return nil

	case StageWorkflow:
		actorLabel := "Customer"
		if len(bundle.Journey.Actors) > 0 {
			actorLabel = bundle.Journey.Actors[0].Label
		}

		lanes := []WorkflowLane{
			{ID: "lane-actor", Label: actorLabel, Order: 1},
			{ID: "lane-system", Label: "System Engine", Order: 2},
		}

		nodes := []WorkflowNode{
			{ID: "wnode-start", LaneID: "lane-actor", Type: "start", Label: "Start Flow", Description: "Initiate workflow", Layout: LayoutCoordinate{X: 50, Y: 50}},
			{ID: "wnode-submit", LaneID: "lane-actor", Type: "action", Label: "Submit Request", Description: "Submit request payload", Layout: LayoutCoordinate{X: 150, Y: 50}},
			{ID: "wnode-decision", LaneID: "lane-system", Type: "decision", Label: "Validate Data", Description: "Check business rules", Layout: LayoutCoordinate{X: 250, Y: 150}},
			{ID: "wnode-process", LaneID: "lane-system", Type: "system_action", Label: "Process Transaction", Description: "Execute core transaction", Layout: LayoutCoordinate{X: 350, Y: 150}},
			{ID: "wnode-reject", LaneID: "lane-system", Type: "system_action", Label: "Reject Submission", Description: "Reject with validation error", Layout: LayoutCoordinate{X: 350, Y: 250}},
			{ID: "wnode-end", LaneID: "lane-system", Type: "end", Label: "End Flow", Description: "Workflow termination", Layout: LayoutCoordinate{X: 450, Y: 150}},
		}

		edges := []WorkflowEdge{
			{ID: "wedge-1", Source: "wnode-start", Target: "wnode-submit", Label: ""},
			{ID: "wedge-2", Source: "wnode-submit", Target: "wnode-decision", Label: ""},
			{ID: "wedge-3", Source: "wnode-decision", Target: "wnode-process", Label: "Valid"},
			{ID: "wedge-4", Source: "wnode-decision", Target: "wnode-reject", Label: "Invalid"},
			{ID: "wedge-5", Source: "wnode-process", Target: "wnode-end", Label: "Success"},
			{ID: "wedge-6", Source: "wnode-reject", Target: "wnode-end", Label: "Failure"},
		}

		bundle.Workflows = []Workflow{
			{
				ID:          "wf-core",
				Name:        "Core Transaction Processing",
				Description: "End-to-end user and system operational workflow",
				Lanes:       lanes,
				Nodes:       nodes,
				Edges:       edges,
			},
		}
		return nil

	case StageWBS:
		modules := []WBSModule{
			{
				ID:          "mod-frontend",
				Name:        "Client Interface Module",
				Description: "Web portal UI and mobile application screens",
				Priority:    "high",
				References:  []string{"architecture.node.node-web"},
				Features: []WBSFeature{
					{
						ID:                 "feat-ui-portal",
						Name:               "Web Portal Interface",
						Description:        "Customer dashboard and interactive submission forms",
						AcceptanceCriteria: []string{"Responsive UI layout", "Form input validation"},
					},
				},
			},
			{
				ID:           "mod-backend",
				Name:         "Application Services Module",
				Description:  "Microservices API and transaction business engine",
				Priority:     "high",
				Dependencies: []string{"mod-frontend"},
				References:   []string{"architecture.node.node-api"},
				Features: []WBSFeature{
					{
						ID:                 "feat-api-services",
						Name:               "Transaction Engine API",
						Description:        "REST API endpoints handling workflow execution",
						AcceptanceCriteria: []string{"Response latency < 200ms", "JSON payload parsing"},
					},
				},
			},
			{
				ID:           "mod-storage",
				Name:         "Data Persistence Module",
				Description:  "Relational database schema and migration scripts",
				Priority:     "medium",
				Dependencies: []string{"mod-backend"},
				References:   []string{"architecture.node.node-db"},
				Features: []WBSFeature{
					{
						ID:                 "feat-db-schema",
						Name:               "Relational Data Models",
						Description:        "Tables, indexes, and transactional consistency",
						AcceptanceCriteria: []string{"Foreign key integrity", "ACID transactions"},
					},
				},
			},
		}

		xrefs := []CrossReference{
			{
				ID:           "xref-1",
				SourceRef:    "journey.activity.act-1",
				TargetRef:    "workflow.wf-core.step.wnode-submit",
				RelationType: "implements",
				Description:  "Activity implemented by workflow action step",
			},
		}
		if len(bundle.Journey.Activities) > 1 {
			xrefs = append(xrefs, CrossReference{
				ID:           "xref-2",
				SourceRef:    "journey.activity.act-2",
				TargetRef:    "workflow.wf-core.step.wnode-process",
				RelationType: "implements",
				Description:  "Settlement activity maps to process node",
			})
		}

		bundle.WBS = WBSSection{Modules: modules}
		bundle.CrossReferences = xrefs
		return nil

	case StageCrossValidation:
		return nil

	default:
		return fmt.Errorf("unknown stage %q", stage)
	}
}
