package service

import (
	"context"
	"errors"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type SkillRepo interface {
	GetActiveSkillByCommand(ctx context.Context, companyID, command string) (*domain.Skill, error)
	Create(ctx context.Context, q repository.Execer, skill *domain.Skill) error
	Update(ctx context.Context, q repository.Execer, skill *domain.Skill) error
}

type SkillService struct {
	repo SkillRepo
}

func NewSkillService(repo SkillRepo) *SkillService {
	return &SkillService{repo: repo}
}

func (s *SkillService) Create(ctx context.Context, q repository.Execer, skill *domain.Skill) error {
	skill.Command = strings.ToLower(skill.Command)
	if !strings.HasPrefix(skill.Command, "/") {
		skill.Command = "/" + skill.Command
	}

	existing, err := s.repo.GetActiveSkillByCommand(ctx, skill.CompanyID.String(), skill.Command)
	if err == nil && existing != nil {
		return errors.New("active skill with this command already exists")
	}
	if err != nil && !errors.Is(err, repository.ErrSkillNotFound) {
		return err
	}

	return s.repo.Create(ctx, q, skill)
}

func (s *SkillService) Update(ctx context.Context, q repository.Execer, skill *domain.Skill) error {
	skill.Command = strings.ToLower(skill.Command)
	if !strings.HasPrefix(skill.Command, "/") {
		skill.Command = "/" + skill.Command
	}

	existing, err := s.repo.GetActiveSkillByCommand(ctx, skill.CompanyID.String(), skill.Command)
	if err == nil && existing != nil && existing.ID != skill.ID {
		return errors.New("active skill with this command already exists")
	}
	if err != nil && !errors.Is(err, repository.ErrSkillNotFound) {
		return err
	}

	return s.repo.Update(ctx, q, skill)
}
