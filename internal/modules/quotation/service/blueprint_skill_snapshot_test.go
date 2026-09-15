package service_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestBuildBlueprintSkillSnapshotProducesAllowlistedImmutableJSON(t *testing.T) {
	fixedID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fixedTime := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	companyID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	skill := &domain.Skill{
		ID:                 fixedID,
		CompanyID:          companyID,
		Name:               "  Blueprint Architect  ",
		Command:            "/blueprint",
		ContentInstruction: "  Follow blueprint template  ",
		IsActive:           true,
		UpdatedAt:          fixedTime,
	}

	snapshot, err := service.BuildBlueprintSkillSnapshot(skill)
	require.NoError(t, err)
	require.True(t, json.Valid(snapshot), "snapshot must be valid JSON")

	var parsed map[string]interface{}
	err = json.Unmarshal(snapshot, &parsed)
	require.NoError(t, err)

	require.Len(t, parsed, 5, "snapshot must contain exactly 5 keys")
	expectedKeys := []string{"id", "name", "command", "content_instruction", "updated_at"}
	for _, key := range expectedKeys {
		_, ok := parsed[key]
		assert.Truef(t, ok, "key %q must be present in snapshot", key)
	}

	assert.Equal(t, fixedID.String(), parsed["id"])
	assert.Equal(t, "Blueprint Architect", parsed["name"])
	assert.Equal(t, "/blueprint", parsed["command"])
	assert.Equal(t, "Follow blueprint template", parsed["content_instruction"])
	assert.Equal(t, fixedTime.Format(time.RFC3339Nano), parsed["updated_at"])

	// Byte equality over two identical calls
	snapshot2, err := service.BuildBlueprintSkillSnapshot(skill)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(snapshot, snapshot2), "two identical calls must produce identical bytes")

	// Immutability: mutate source domain.Skill fields and ensure snapshot is not altered
	skill.ID = uuid.New()
	skill.CompanyID = uuid.New()
	skill.Name = "MUTATED NAME"
	skill.Command = "/other"
	skill.ContentInstruction = "MUTATED INSTRUCTION"
	skill.IsActive = false
	skill.UpdatedAt = time.Now().Add(24 * time.Hour)
	now := time.Now()
	skill.DeletedAt = &now

	assert.True(t, bytes.Equal(snapshot, snapshot2), "snapshot raw bytes must remain unchanged after source mutation")

	var parsedAfterMutation map[string]interface{}
	err = json.Unmarshal(snapshot, &parsedAfterMutation)
	require.NoError(t, err)
	assert.Equal(t, fixedID.String(), parsedAfterMutation["id"])
	assert.Equal(t, "Blueprint Architect", parsedAfterMutation["name"])
	assert.Equal(t, "/blueprint", parsedAfterMutation["command"])
	assert.Equal(t, "Follow blueprint template", parsedAfterMutation["content_instruction"])
}

func TestBuildBlueprintSkillSnapshotRejectsInvalidSkill(t *testing.T) {
	validSkill := func() *domain.Skill {
		return &domain.Skill{
			ID:                 uuid.New(),
			CompanyID:          uuid.New(),
			Name:               "Blueprint Skill",
			Command:            service.BlueprintSkillCommand,
			ContentInstruction: "Instruction",
			IsActive:           true,
			UpdatedAt:          time.Now().UTC(),
		}
	}

	now := time.Now().UTC()

	tests := []struct {
		name  string
		skill *domain.Skill
	}{
		{
			name:  "nil skill",
			skill: nil,
		},
		{
			name: "zero ID",
			skill: func() *domain.Skill {
				s := validSkill()
				s.ID = uuid.Nil
				return s
			}(),
		},
		{
			name: "blank name",
			skill: func() *domain.Skill {
				s := validSkill()
				s.Name = "   "
				return s
			}(),
		},
		{
			name: "wrong command",
			skill: func() *domain.Skill {
				s := validSkill()
				s.Command = "/quotation"
				return s
			}(),
		},
		{
			name: "blank command",
			skill: func() *domain.Skill {
				s := validSkill()
				s.Command = "   "
				return s
			}(),
		},
		{
			name: "blank instruction",
			skill: func() *domain.Skill {
				s := validSkill()
				s.ContentInstruction = "   "
				return s
			}(),
		},
		{
			name: "inactive",
			skill: func() *domain.Skill {
				s := validSkill()
				s.IsActive = false
				return s
			}(),
		},
		{
			name: "non-nil DeletedAt",
			skill: func() *domain.Skill {
				s := validSkill()
				s.DeletedAt = &now
				return s
			}(),
		},
		{
			name: "zero UpdatedAt",
			skill: func() *domain.Skill {
				s := validSkill()
				s.UpdatedAt = time.Time{}
				return s
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := service.BuildBlueprintSkillSnapshot(tc.skill)
			require.Error(t, err)
			assert.True(t, errors.Is(err, service.ErrBlueprintSkillSnapshotInvalid),
				"expected ErrBlueprintSkillSnapshotInvalid, got: %v", err)
			assert.Nil(t, snapshot)
		})
	}
}

func TestBuildBlueprintSkillSnapshotDoesNotLeakMetadataOrSecrets(t *testing.T) {
	createdByID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	companyID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	createdAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	skill := &domain.Skill{
		ID:                 uuid.New(),
		CompanyID:          companyID,
		Name:               "Blueprint Generator",
		Command:            service.BlueprintSkillCommand,
		ContentInstruction: "Draft blueprint according to client requirements",
		IsActive:           true,
		CreatedBy:          &createdByID,
		CreatedAt:          createdAt,
		UpdatedAt:          time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
	}

	snapshot, err := service.BuildBlueprintSkillSnapshot(skill)
	require.NoError(t, err)

	snapshotStr := string(snapshot)

	forbiddenSubstrings := []string{
		"company_id",
		"created_by",
		"created_at",
		"deleted_at",
		"is_active",
		"api_key",
		"credential",
		"token",
		"provider",
		"model",
		companyID.String(),
		createdByID.String(),
	}

	for _, forbidden := range forbiddenSubstrings {
		assert.Falsef(t, strings.Contains(strings.ToLower(snapshotStr), strings.ToLower(forbidden)),
			"snapshot leaked forbidden token %q in JSON: %s", forbidden, snapshotStr)
	}
}
