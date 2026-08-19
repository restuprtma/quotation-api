package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/modules/quotation/rnd"
)

const (
	defaultQuotationArtifacts = "../docs/roadmap/rnd/quotation-skill"
	defaultBlueprintArtifacts = "../docs/roadmap/rnd/blueprint-skill"
)

type options struct {
	Skill, Fixture, Provider, Model, Output, Artifacts string
	Runs                                               int
}

type optionDefaults struct {
	Provider, Model, MockModel, OllamaModel, GeminiModel, AgentRouterModel, Artifacts string
}

type runnerAPI interface {
	Run(context.Context, string) (*rnd.BatchReport, error)
}

type blueprintRunnerAPI interface {
	Run(context.Context, string) (*rnd.BlueprintBatchReport, error)
}

var (
	configLoader    = config.Load
	providerFactory = func(name string, providerConfig rnd.ProviderConfig) (rnd.Provider, error) {
		return rnd.NewProvider(name, providerConfig, nil)
	}
	runnerFactory = func(runnerConfig rnd.RunnerConfig) (runnerAPI, error) {
		return rnd.NewRunner(runnerConfig)
	}
	blueprintRunnerFactory = func(runnerConfig rnd.BlueprintRunnerConfig) (blueprintRunnerAPI, error) {
		return rnd.NewBlueprintRunner(runnerConfig)
	}
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func parseOptions(args []string, defaults optionDefaults) (options, error) {
	value := options{Provider: defaults.Provider, Model: defaults.Model, Artifacts: defaults.Artifacts, Runs: 2}
	flags := flag.NewFlagSet("skill-rnd", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&value.Skill, "skill", "", "skill name")
	flags.StringVar(&value.Fixture, "fixture", "", "fixture path")
	flags.StringVar(&value.Provider, "provider", value.Provider, "provider name")
	flags.StringVar(&value.Model, "model", value.Model, "model name")
	flags.IntVar(&value.Runs, "runs", value.Runs, "run count")
	flags.StringVar(&value.Output, "output", "", "sanitized output directory")
	flags.StringVar(&value.Artifacts, "artifacts", value.Artifacts, "artifact root")
	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	if value.Skill == "" {
		return options{}, errors.New("--skill is required")
	}
	if value.Skill != "quotation" && value.Skill != "blueprint" {
		return options{}, errors.New("--skill must be quotation or blueprint")
	}
	if strings.TrimSpace(value.Fixture) == "" {
		return options{}, errors.New("--fixture is required")
	}
	if !contains([]string{"mock", "ollama", "gemini", "agentrouter"}, value.Provider) {
		return options{}, errors.New("--provider must be mock, ollama, gemini, or agentrouter")
	}
	modelExplicit := false
	artifactsExplicit := false
	flags.Visit(func(item *flag.Flag) {
		if item.Name == "model" {
			modelExplicit = true
		}
		if item.Name == "artifacts" {
			artifactsExplicit = true
		}
	})
	if !modelExplicit && value.Provider != defaults.Provider {
		switch value.Provider {
		case "mock":
			value.Model = defaults.MockModel
		case "ollama":
			value.Model = defaults.OllamaModel
		case "gemini":
			value.Model = defaults.GeminiModel
		case "agentrouter":
			value.Model = defaults.AgentRouterModel
		}
	}
	if strings.TrimSpace(value.Model) == "" {
		return options{}, errors.New("--model is required after provider resolution")
	}
	if value.Runs < 1 || value.Runs > 10 {
		return options{}, errors.New("--runs must be between 1 and 10")
	}
	if !artifactsExplicit && strings.TrimSpace(defaults.Artifacts) == "" {
		switch value.Skill {
		case "blueprint":
			value.Artifacts = defaultBlueprintArtifacts
		case "quotation":
			value.Artifacts = defaultQuotationArtifacts
		}
	}
	if strings.TrimSpace(value.Artifacts) == "" {
		return options{}, errors.New("--artifacts must not be empty")
	}
	return value, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	configuration := configLoader()
	defaults := optionDefaults{
		Provider: configuration.AI.Provider, MockModel: "deterministic-rnd-v1",
		OllamaModel: configuration.AI.OllamaModel, GeminiModel: configuration.AI.GeminiModel, AgentRouterModel: configuration.AI.AgentRouterModel,
		Artifacts: "",
	}
	switch defaults.Provider {
	case "ollama":
		defaults.Model = defaults.OllamaModel
	case "gemini":
		defaults.Model = defaults.GeminiModel
	case "agentrouter":
		defaults.Model = defaults.AgentRouterModel
	default:
		defaults.Model = defaults.MockModel
	}
	value, err := parseOptions(args, defaults)
	if err != nil {
		fmt.Fprintln(stderr, "error="+err.Error())
		return 2
	}
	artifacts, err := filepath.Abs(value.Artifacts)
	if err != nil {
		fmt.Fprintln(stderr, "error=invalid artifact path")
		return 1
	}
	output, err := prepareOutput(value.Output, artifacts)
	if err != nil {
		fmt.Fprintln(stderr, "error="+err.Error())
		return 1
	}
	rawDir, err := os.MkdirTemp("", fmt.Sprintf("%s-skill-rnd-raw-*", value.Skill))
	if err != nil {
		fmt.Fprintln(stderr, "error=failed to create protected storage")
		return 1
	}
	if err := os.Chmod(rawDir, 0o700); err != nil {
		fmt.Fprintln(stderr, "error=failed to protect storage")
		return 1
	}
	provider, err := providerFactory(value.Provider, rnd.ProviderConfig{OllamaBaseURL: configuration.AI.OllamaBaseURL, GeminiAPIKey: configuration.AI.GeminiAPIKey, AgentRouterBaseURL: configuration.AI.AgentRouterBaseURL, AgentRouterAPIKey: configuration.AI.AgentRouterAPIKey, Timeout: configuration.AI.Timeout})
	if err != nil {
		fmt.Fprintln(stderr, "error=provider configuration failed")
		return 1
	}

	if value.Skill == "blueprint" {
		runner, err := blueprintRunnerFactory(rnd.BlueprintRunnerConfig{ArtifactRoot: artifacts, OutputDir: output, RawDir: rawDir, Provider: provider, Model: value.Model, Runs: value.Runs})
		if err != nil {
			fmt.Fprintln(stderr, "error=runner initialization failed")
			return 1
		}
		report, err := runner.Run(ctx, value.Fixture)
		if err != nil || report == nil {
			fmt.Fprintln(stderr, "error=blueprint R&D run failed")
			return 1
		}
		reportPath := filepath.Join(output, fmt.Sprintf("%s-report.json", report.Fixture))
		fmt.Fprintln(stdout, "status="+report.Status)
		fmt.Fprintln(stdout, "output="+output)
		fmt.Fprintln(stdout, "report="+reportPath)
		if report.Status != "pass" {
			return 1
		}
		return 0
	}

	runner, err := runnerFactory(rnd.RunnerConfig{ArtifactRoot: artifacts, OutputDir: output, RawDir: rawDir, Provider: provider, Model: value.Model, Runs: value.Runs})
	if err != nil {
		if errors.Is(err, rnd.ErrLegalTemplateUnapproved) {
			fmt.Fprintln(stderr, "error="+rnd.ErrLegalTemplateUnapproved.Error())
		} else {
			fmt.Fprintln(stderr, "error=runner initialization failed")
		}
		return 1
	}
	report, err := runner.Run(ctx, value.Fixture)
	if err != nil || report == nil {
		fmt.Fprintln(stderr, "error=quotation R&D run failed")
		return 1
	}
	reportPath, err := rnd.ReportPath(output, report.Fixture)
	if err != nil {
		fmt.Fprintln(stderr, "error=report path resolution failed")
		return 1
	}
	fmt.Fprintln(stdout, "status="+report.Aggregate.Status)
	fmt.Fprintln(stdout, "output="+output)
	fmt.Fprintln(stdout, "report="+reportPath)
	if report.Aggregate.Status != "pass" {
		return 1
	}
	return 0
}

func prepareOutput(requested, artifactRoot string) (string, error) {
	if requested == "" {
		path, err := os.MkdirTemp("", "quotation-skill-rnd-output-*")
		if err != nil {
			return "", errors.New("failed to create output directory")
		}
		if err := os.Chmod(path, 0o750); err != nil {
			return "", errors.New("failed to protect output directory")
		}
		return filepath.Abs(path)
	}
	output, err := filepath.Abs(requested)
	if err != nil {
		return "", errors.New("invalid output path")
	}
	inside, err := pathWithin(artifactRoot, output)
	if err != nil {
		return "", errors.New("failed to validate output path")
	}
	if inside {
		return "", errors.New("output must be outside artifact root")
	}
	if err := os.MkdirAll(output, 0o750); err != nil {
		return "", errors.New("failed to create output directory")
	}
	if err := os.Chmod(output, 0o750); err != nil {
		return "", errors.New("failed to protect output directory")
	}
	return output, nil
}

func pathWithin(parent, child string) (bool, error) {
	parent, err := resolvedPath(parent)
	if err != nil {
		return false, err
	}
	child, err = resolvedProspectivePath(child)
	if err != nil {
		return false, err
	}
	if runtime.GOOS == "windows" {
		parent, child = strings.ToLower(parent), strings.ToLower(child)
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false, nil
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)), nil
}

func resolvedPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func resolvedProspectivePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(absolute), nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
