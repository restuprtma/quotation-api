package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrMergeOwnershipCollision is a configuration/programming error. It must
// never be downgraded to a user-facing warning because a phase would then be
// allowed to silently overwrite a different phase's field.
var ErrMergeOwnershipCollision = errors.New("meeting extraction merge ownership collision")

type MergeSource string

const (
	MergeSourceUser      MergeSource = "user"
	MergeSourceExtracted MergeSource = "extracted"
	MergeSourceDefault   MergeSource = "default"
)

// MergeProvenance deliberately retains only source identifiers and the safe
// evidence token. It must not contain canonical-note text or a provider reply.
type MergeProvenance struct {
	Phase         PhaseKey
	Support       string
	Source        MergeSource
	EvidenceRefs  []string
	EvidenceToken string
}

type MergedValue struct {
	Value      any
	Provenance MergeProvenance
}

// MergeCandidate is the normalized, phase-owned value presented to P5. The
// evidence result has already been reconstructed by the evidence checker; the
// merge never accepts provider text as evidence.
type MergeCandidate struct {
	Path          FieldPath
	Value         any
	Phase         PhaseKey
	Source        MergeSource
	Support       string
	EvidenceRefs  []string
	EvidenceToken string
	// EvidenceValid is set only after the evidence checker accepted the
	// candidate. User and backend-default values do not require it.
	EvidenceValid bool
	Required      bool
}

// MergeConsistencyObservation is descriptive evidence from another phase
// (for example a P4 timeline) and never transfers ownership. It can only
// invalidate an inconsistent canonical value and request user input.
type MergeConsistencyObservation struct {
	Path          FieldPath
	Value         any
	Phase         PhaseKey
	Source        MergeSource
	Support       string
	EvidenceRefs  []string
	EvidenceToken string
	EvidenceValid bool
}

type MergeWarningCandidate struct {
	Value      any
	Provenance MergeProvenance
	// SourceText is permanently empty. Its presence makes the no-quote contract
	// explicit for future API/status projections.
	SourceText string
}

type MergeWarning struct {
	Code       string
	Field      FieldPath
	Candidates []MergeWarningCandidate
}

type MergeExtractionInput struct {
	Phases         []Phase
	Candidates     []MergeCandidate
	Consistency    []MergeConsistencyObservation
	RepairAttempts map[FieldPath]int
}

type MergeExtractionResult struct {
	Values       map[FieldPath]MergedValue
	Warnings     []MergeWarning
	NeedsInput   bool
	RepairFields []FieldPath
}

// NewExtractedTextMergeCandidate is the bridge from M7.1 to the deterministic
// merger. Invalid evidence is deliberately represented as an invalid
// candidate, rather than returned as a raw provider error, so MergeExtraction
// can drop only that field, emit a safe warning, and apply the one-repair rule.
func NewExtractedTextMergeCandidate(checker *MeetingEvidenceChecker, path FieldPath, phase PhaseKey, value string, evidence MeetingEvidence, required bool) MergeCandidate {
	candidate := MergeCandidate{
		Path:         path,
		Value:        value,
		Phase:        phase,
		Source:       MergeSourceExtracted,
		Support:      evidence.Support,
		EvidenceRefs: append([]string(nil), evidence.Refs...),
		Required:     required,
	}
	if checker == nil {
		return candidate
	}
	validated, err := checker.ValidateText(evidence, value)
	if err != nil {
		return candidate
	}
	candidate.EvidenceValid = true
	candidate.Support = validated.Support
	candidate.EvidenceToken = validated.Token
	return candidate
}

func (r MergeExtractionResult) WarningCodes() []string {
	codes := make([]string, 0, len(r.Warnings))
	for _, warning := range r.Warnings {
		codes = append(codes, warning.Code)
	}
	return codes
}

// MergeExtraction applies the fixed precedence user > extracted > default.
// Ownership must be valid before values are considered. An evidence failure
// removes only the affected extracted value and allows no more than one repair
// request for each required field.
func MergeExtraction(in MergeExtractionInput) (MergeExtractionResult, error) {
	if err := validateMergeOwnership(in.Phases); err != nil {
		return MergeExtractionResult{}, err
	}
	result := MergeExtractionResult{Values: make(map[FieldPath]MergedValue)}
	byPath := make(map[FieldPath][]MergeCandidate)
	for _, candidate := range in.Candidates {
		if strings.TrimSpace(string(candidate.Path)) == "" {
			return MergeExtractionResult{}, fmt.Errorf("%w: empty field path", ErrMergeOwnershipCollision)
		}
		byPath[candidate.Path] = append(byPath[candidate.Path], candidate)
	}
	paths := sortedMergePaths(byPath)
	for _, path := range paths {
		candidates := byPath[path]
		valid := make([]MergeCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Source == MergeSourceExtracted && !candidate.EvidenceValid {
				result.Warnings = append(result.Warnings, MergeWarning{Code: "EVIDENCE_MISMATCH", Field: path})
				if candidate.Required {
					if in.RepairAttempts[path] < 1 {
						result.RepairFields = appendUniqueField(result.RepairFields, path)
					} else {
						result.NeedsInput = true
					}
				}
				continue
			}
			if !validMergeSource(candidate.Source) {
				return MergeExtractionResult{}, fmt.Errorf("%w: invalid merge source", ErrMergeOwnershipCollision)
			}
			valid = append(valid, candidate)
		}
		if chosen, ok := selectMergeCandidate(valid); ok {
			result.Values[path] = MergedValue{Value: chosen.Value, Provenance: mergeProvenance(chosen.Phase, chosen.Support, chosen.Source, chosen.EvidenceRefs, chosen.EvidenceToken)}
		}
	}

	for _, observation := range in.Consistency {
		if observation.Source == MergeSourceExtracted && !observation.EvidenceValid {
			result.Warnings = append(result.Warnings, MergeWarning{Code: "EVIDENCE_MISMATCH", Field: observation.Path})
			continue
		}
		canonical, exists := result.Values[observation.Path]
		// A semantic observation is not an owner. It therefore cannot displace
		// an explicit user choice or a backend default; it only cross-checks the
		// extracted canonical fact owned by a different phase.
		if !exists || canonical.Provenance.Source != MergeSourceExtracted || equalMergeValue(canonical.Value, observation.Value) {
			continue
		}
		delete(result.Values, observation.Path)
		result.NeedsInput = true
		result.Warnings = append(result.Warnings, MergeWarning{
			Code:  "CONFLICT",
			Field: observation.Path,
			Candidates: sortedWarningCandidates([]MergeWarningCandidate{
				{Value: canonical.Value, Provenance: canonical.Provenance},
				{Value: observation.Value, Provenance: mergeProvenance(observation.Phase, observation.Support, observation.Source, observation.EvidenceRefs, observation.EvidenceToken)},
			}),
		})
	}
	sort.Slice(result.RepairFields, func(i, j int) bool { return result.RepairFields[i] < result.RepairFields[j] })
	return result, nil
}

func validateMergeOwnership(phases []Phase) error {
	owners := make(map[FieldPath]PhaseKey)
	for _, phase := range phases {
		if phase == nil {
			return fmt.Errorf("%w: nil phase", ErrMergeOwnershipCollision)
		}
		for _, field := range phase.Owns() {
			if previous, exists := owners[field]; exists {
				return fmt.Errorf("%w: %s claimed by %s and %s", ErrMergeOwnershipCollision, field, previous, phase.Key())
			}
			owners[field] = phase.Key()
		}
	}
	return nil
}

func validMergeSource(source MergeSource) bool {
	return source == MergeSourceUser || source == MergeSourceExtracted || source == MergeSourceDefault
}

func selectMergeCandidate(candidates []MergeCandidate) (MergeCandidate, bool) {
	if len(candidates) == 0 {
		return MergeCandidate{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := mergeSourceRank(candidates[i].Source), mergeSourceRank(candidates[j].Source)
		if left != right {
			return left < right
		}
		if candidates[i].Phase != candidates[j].Phase {
			return candidates[i].Phase < candidates[j].Phase
		}
		return fmt.Sprint(candidates[i].Value) < fmt.Sprint(candidates[j].Value)
	})
	return candidates[0], true
}

func mergeSourceRank(source MergeSource) int {
	switch source {
	case MergeSourceUser:
		return 0
	case MergeSourceExtracted:
		return 1
	default:
		return 2
	}
}

func mergeProvenance(phase PhaseKey, support string, source MergeSource, refs []string, token string) MergeProvenance {
	refs = append([]string(nil), refs...)
	sort.Strings(refs)
	return MergeProvenance{Phase: phase, Support: support, Source: source, EvidenceRefs: refs, EvidenceToken: token}
}

func sortedMergePaths(values map[FieldPath][]MergeCandidate) []FieldPath {
	paths := make([]FieldPath, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
	return paths
}

func appendUniqueField(values []FieldPath, path FieldPath) []FieldPath {
	for _, value := range values {
		if value == path {
			return values
		}
	}
	return append(values, path)
}

func equalMergeValue(left, right any) bool {
	return fmt.Sprintf("%#v", left) == fmt.Sprintf("%#v", right)
}

func sortedWarningCandidates(values []MergeWarningCandidate) []MergeWarningCandidate {
	sort.Slice(values, func(i, j int) bool {
		left, right := fmt.Sprint(values[i].Value), fmt.Sprint(values[j].Value)
		if left != right {
			return left < right
		}
		return values[i].Provenance.Phase < values[j].Provenance.Phase
	})
	return values
}
