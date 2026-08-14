package service

import (
	"errors"
	"testing"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func TestValidateAIBundle(t *testing.T) {
	validBundle := func() *dto.AIGenerateResponse {
		return &dto.AIGenerateResponse{
			SchemaVersion: "1.0",
			Narrative: dto.AINarrative{
				Introduction: "Hello",
			},
			ProductOrProjectDetail: dto.AIProductOrProjectDetail{
				Mode: "product",
			},
			Blueprint: dto.AIBlueprint{
				Diagrams: []dto.AIDiagram{
					{Type: "journey"},
				},
			},
		}
	}

	t.Run("valid bundle", func(t *testing.T) {
		err := ValidateAIBundle(validBundle())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing schema version", func(t *testing.T) {
		b := validBundle()
		b.SchemaVersion = ""
		err := ValidateAIBundle(b)
		if !errors.Is(err, ErrInvalidAIBundle) {
			t.Errorf("expected ErrInvalidAIBundle, got %v", err)
		}
	})

	t.Run("invalid mode", func(t *testing.T) {
		b := validBundle()
		b.ProductOrProjectDetail.Mode = "invalid"
		err := ValidateAIBundle(b)
		if !errors.Is(err, ErrInvalidAIBundle) {
			t.Errorf("expected ErrInvalidAIBundle, got %v", err)
		}
	})

	t.Run("unsupported diagram type", func(t *testing.T) {
		b := validBundle()
		b.Blueprint.Diagrams = append(b.Blueprint.Diagrams, dto.AIDiagram{Type: "invalid_type"})
		err := ValidateAIBundle(b)
		if !errors.Is(err, ErrInvalidAIBundle) {
			t.Errorf("expected ErrInvalidAIBundle, got %v", err)
		}
	})

	t.Run("unsafe html injection", func(t *testing.T) {
		b := validBundle()
		b.Narrative.Introduction = "This is safe <script>alert(1)</script>"
		err := ValidateAIBundle(b)
		if !errors.Is(err, ErrUnsafeAIBundle) {
			t.Errorf("expected ErrUnsafeAIBundle, got %v", err)
		}
	})

	t.Run("too many diagrams", func(t *testing.T) {
		b := validBundle()
		for i := 0; i < 11; i++ {
			b.Blueprint.Diagrams = append(b.Blueprint.Diagrams, dto.AIDiagram{Type: "journey"})
		}
		err := ValidateAIBundle(b)
		if !errors.Is(err, ErrInvalidAIBundle) {
			t.Errorf("expected ErrInvalidAIBundle, got %v", err)
		}
	})
}
