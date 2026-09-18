package dto

import "strings"

// BlueprintSourceMetadata represents metadata describing whether a revision is eligible to serve as a blueprint source.
type BlueprintSourceMetadata struct {
	BlueprintEligible         bool    `json:"blueprint_eligible"`
	BlueprintIneligibleReason *string `json:"blueprint_ineligible_reason"`
}

// EvaluateBlueprintSourceEligibility evaluates whether a quotation revision can be used as a blueprint generation source.
func EvaluateBlueprintSourceEligibility(pubStatus, genStatus string, contentHTML *string) BlueprintSourceMetadata {
	switch pubStatus {
	case "draft", "published", "superseded":
	default:
		reason := "unsupported publication status"
		return BlueprintSourceMetadata{
			BlueprintEligible:         false,
			BlueprintIneligibleReason: &reason,
		}
	}

	if genStatus != "completed" {
		reason := "revision generation is incomplete"
		return BlueprintSourceMetadata{
			BlueprintEligible:         false,
			BlueprintIneligibleReason: &reason,
		}
	}

	if contentHTML == nil || strings.TrimSpace(*contentHTML) == "" {
		reason := "revision content is blank"
		return BlueprintSourceMetadata{
			BlueprintEligible:         false,
			BlueprintIneligibleReason: &reason,
		}
	}

	return BlueprintSourceMetadata{
		BlueprintEligible:         true,
		BlueprintIneligibleReason: nil,
	}
}
