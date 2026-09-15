package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const BlueprintSkillCommand = "/blueprint"

var ErrBlueprintSkillSnapshotInvalid = errors.New("invalid blueprint skill snapshot input")

type BlueprintSkillSnapshot struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Command            string    `json:"command"`
	ContentInstruction string    `json:"content_instruction"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func BuildBlueprintSkillSnapshot(skill *domain.Skill) (json.RawMessage, error) {
	if skill == nil {
		return nil, fmt.Errorf("%w: skill is nil", ErrBlueprintSkillSnapshotInvalid)
	}
	if skill.ID == uuid.Nil {
		return nil, fmt.Errorf("%w: skill id is zero", ErrBlueprintSkillSnapshotInvalid)
	}
	if strings.TrimSpace(skill.Name) == "" {
		return nil, fmt.Errorf("%w: skill name is empty", ErrBlueprintSkillSnapshotInvalid)
	}
	if skill.Command != BlueprintSkillCommand {
		return nil, fmt.Errorf("%w: skill command is invalid", ErrBlueprintSkillSnapshotInvalid)
	}
	if strings.TrimSpace(skill.ContentInstruction) == "" {
		return nil, fmt.Errorf("%w: skill content instruction is empty", ErrBlueprintSkillSnapshotInvalid)
	}
	if !skill.IsActive {
		return nil, fmt.Errorf("%w: skill is not active", ErrBlueprintSkillSnapshotInvalid)
	}
	if skill.DeletedAt != nil {
		return nil, fmt.Errorf("%w: skill is deleted", ErrBlueprintSkillSnapshotInvalid)
	}
	if skill.UpdatedAt.IsZero() {
		return nil, fmt.Errorf("%w: skill updated_at is zero", ErrBlueprintSkillSnapshotInvalid)
	}

	snapshot := BlueprintSkillSnapshot{
		ID:                 skill.ID,
		Name:               strings.TrimSpace(skill.Name),
		Command:            skill.Command,
		ContentInstruction: strings.TrimSpace(skill.ContentInstruction),
		UpdatedAt:          skill.UpdatedAt.UTC(),
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal snapshot", ErrBlueprintSkillSnapshotInvalid)
	}

	return json.RawMessage(payload), nil
}
