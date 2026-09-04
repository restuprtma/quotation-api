package service

import (
	"strings"
	"testing"
)

func TestAllocationTaskSchema_NoFullDurationExitHatch(t *testing.T) {
	removed := []string{
		"roles may overlap for the full duration",
		"phase separation is not meaningful",
	}
	for _, marker := range removed {
		if strings.Contains(allocationTaskSchema, marker) {
			t.Errorf("allocationTaskSchema still contains removed exit-hatch text %q", marker)
		}
	}
}

func TestAllocationTaskSchema_MandatoryLadderPresent(t *testing.T) {
	required := []string{
		"HARD CONSTRAINT",
		"Do NOT default every role to the full project duration",
		"LEVEL 1",
		"LEVEL 2",
		"LEVEL 3",
		"LEVEL 4",
		"budget_unfeasible",
		"minimum viable team",
	}
	for _, marker := range required {
		if !strings.Contains(allocationTaskSchema, marker) {
			t.Errorf("allocationTaskSchema is missing mandatory marker %q", marker)
		}
	}
}

func TestDedicatedSquadModulePrompt_IsEmbeddedAndCommerciallyIsolated(t *testing.T) {
	prompt := strings.ToLower(GetSafetyEnvelope("dedicated_squad_modules_v1.txt"))
	if prompt == "" {
		t.Fatal("dedicated squad module extraction prompt is not embedded")
	}
	for _, marker := range []string{"json only", "price", "layout", "legal", "timeline", "role", "headcount"} {
		if !strings.Contains(prompt, marker) {
			t.Errorf("module extraction prompt is missing isolation marker %q", marker)
		}
	}
}
