package service

import (
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ResolveStandaloneMaintenanceProfile resolves document section plan,
// tax treatment copy, and conditional rights/discount visibility flags for standalone maintenance.
func ResolveStandaloneMaintenanceProfile(input domain.StandaloneMaintenanceInput) domain.ResolvedStandaloneMaintenanceProfile {
	showRights := len(input.RightsStatements) > 0

	profile := domain.ResolvedStandaloneMaintenanceProfile{
		Mode:            input.Mode,
		TaxTreatment:    input.TaxTreatment,
		ShowDiscountRow: input.ShowDiscountRow,
		ShowRights:      showRights,
	}

	switch input.TaxTreatment {
	case domain.StandaloneTaxExclusive:
		profile.TaxClauseText = "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia."
	case domain.StandaloneTaxInclusive:
		profile.TaxClauseText = "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini."
	case domain.StandaloneTaxNotStated:
		profile.TaxClauseText = ""
	default:
		profile.TaxClauseText = ""
	}

	costCode := "maintenance_cost"
	costTitle := "Biaya Maintenance"
	if input.Mode == domain.StandaloneMaintenanceHourlyTask {
		costCode = "development_cost"
		costTitle = "Biaya Development"
	}

	sections := []domain.DocumentSection{
		{Number: 1, Code: costCode, Title: costTitle},
		{Number: 2, Code: "maintenance_scope", Title: "Lingkup Pekerjaan & Timeline"},
		{Number: 3, Code: "payment", Title: "Pembayaran"},
	}

	if showRights {
		sections = append(sections, domain.DocumentSection{
			Number: 4,
			Code:   "rights",
			Title:  "Hak Para Pihak",
		})
		sections = append(sections, domain.DocumentSection{
			Number: 5,
			Code:   "limitations",
			Title:  "Batasan dan Pengecualian",
		})
	} else {
		sections = append(sections, domain.DocumentSection{
			Number: 4,
			Code:   "limitations",
			Title:  "Batasan dan Pengecualian",
		})
	}

	profile.Sections = sections
	return profile
}

// NormalizeStandaloneMaintenanceScope trims all string fields, preserves group/task order,
// converts inactive mode arrays into non-nil empty slices, and preserves priority/reference casing.
func NormalizeStandaloneMaintenanceScope(input domain.StandaloneMaintenanceInput) domain.StandaloneMaintenanceScopeSnapshot {
	snapshot := domain.StandaloneMaintenanceScopeSnapshot{
		DefectGroups: make([]domain.MaintenanceDefectGroupInput, 0),
		HourlyTasks:  make([]domain.MaintenanceHourlyTaskInput, 0),
	}

	switch input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		if len(input.DefectGroups) > 0 {
			snapshot.DefectGroups = make([]domain.MaintenanceDefectGroupInput, 0, len(input.DefectGroups))
			for _, group := range input.DefectGroups {
				normalizedGroup := domain.MaintenanceDefectGroupInput{
					Title: strings.TrimSpace(group.Title),
					Items: make([]domain.MaintenanceDefectItemInput, 0, len(group.Items)),
				}
				for _, item := range group.Items {
					normalizedGroup.Items = append(normalizedGroup.Items, domain.MaintenanceDefectItemInput{
						ItemNumber:  strings.TrimSpace(item.ItemNumber),
						Priority:    item.Priority,
						Function:    strings.TrimSpace(item.Function),
						ScreenFlow:  strings.TrimSpace(item.ScreenFlow),
						Existing:    strings.TrimSpace(item.Existing),
						Improvement: strings.TrimSpace(item.Improvement),
						Reference:   strings.TrimSpace(item.Reference),
					})
				}
				snapshot.DefectGroups = append(snapshot.DefectGroups, normalizedGroup)
			}
		}

	case domain.StandaloneMaintenanceHourlyTask:
		if len(input.HourlyTasks) > 0 {
			snapshot.HourlyTasks = make([]domain.MaintenanceHourlyTaskInput, 0, len(input.HourlyTasks))
			for _, task := range input.HourlyTasks {
				snapshot.HourlyTasks = append(snapshot.HourlyTasks, domain.MaintenanceHourlyTaskInput{
					Description:    strings.TrimSpace(task.Description),
					Platform:       strings.TrimSpace(task.Platform),
					HoursHalfUnits: task.HoursHalfUnits,
				})
			}
		}
	}

	return snapshot
}
