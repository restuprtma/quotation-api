package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var numberedTaskPrefix = regexp.MustCompile(`^\d+[.)]\s*`)

// GenerateTimeline builds deterministic rows. Calendar allocation counts all
// calendar days, while sprint workday labels count Monday-Friday inclusively.
// National holidays are not deducted because this engine has no holiday calendar.
func GenerateTimeline(input domain.DedicatedSquadInput, rules DedicatedSquadRules, modules []domain.Module) ([]domain.TimelineRow, error) {
	if !rules.IncludeTimeline || input.Model == domain.ModelStaffAugmentation {
		return []domain.TimelineRow{}, nil
	}
	if input.Model == domain.ModelSprint {
		return generateSprintTimeline(input)
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("modules: must contain at least one module for project timeline")
	}

	switch input.Timeline.Mode {
	case domain.TimelineRelative:
		return generateRelativeTimeline(rules, modules), nil
	case domain.TimelineCalendar:
		if input.Timeline.StartDate == nil {
			return nil, fmt.Errorf("timeline.start_date: is required for calendar timeline")
		}
		return generateCalendarTimeline(*input.Timeline.StartDate, rules, modules), nil
	default:
		return nil, fmt.Errorf("timeline.mode: unsupported project timeline mode %q", input.Timeline.Mode)
	}
}

func generateRelativeTimeline(rules DedicatedSquadRules, modules []domain.Module) []domain.TimelineRow {
	rows := []domain.TimelineRow{
		phaseRow("Requirement Analysis & System Design", "Bulan 1", PhaseRequirement),
		developmentRow("Bulan 2, 3", modules),
	}
	if rules.IncludeK6 {
		rows = append(rows, phaseRow("Performance / Load Test", "2 Minggu", PhaseK6))
	}
	if rules.IncludePentest {
		rows = append(rows, phaseRow("Penetration Test", "2 Minggu", PhasePentest))
	}
	rows = append(rows,
		phaseRow("System Integration Testing (SIT)", "Bulan 4", PhaseSIT),
		phaseRow("User Acceptance Testing (UAT)", "2 Minggu", PhaseUAT),
		phaseRow("Go-Live", "Bulan 5", PhaseGoLive),
	)
	if rules.IncludeMaintenance {
		rows = append(rows, phaseRow("Support & Maintenance", "Bulan 6 - 17", PhaseMaintenance))
	}
	return rows
}

func generateCalendarTimeline(start time.Time, rules DedicatedSquadRules, modules []domain.Module) []domain.TimelineRow {
	cursor := calendarDate(start)
	requirementEnd := cursor.AddDate(0, 0, 13)
	rows := []domain.TimelineRow{phaseRow("Requirement Analysis & System Design", formatIndonesianRange(cursor, requirementEnd), PhaseRequirement)}

	cursor = requirementEnd.AddDate(0, 0, 1)
	developmentEnd := cursor.AddDate(0, 0, 55)
	rows = append(rows, developmentRow(formatIndonesianRange(cursor, developmentEnd), modules))
	cursor = developmentEnd.AddDate(0, 0, 1)

	if rules.IncludeK6 || rules.IncludePentest {
		securityEnd := cursor.AddDate(0, 0, 13)
		if rules.IncludeK6 {
			rows = append(rows, phaseRow("Performance / Load Test", formatIndonesianRange(cursor, securityEnd), PhaseK6))
		}
		if rules.IncludePentest {
			rows = append(rows, phaseRow("Penetration Test", formatIndonesianRange(cursor, securityEnd), PhasePentest))
		}
		cursor = securityEnd.AddDate(0, 0, 1)
	}

	sitEnd := cursor.AddDate(0, 0, 13)
	rows = append(rows, phaseRow("System Integration Testing (SIT)", formatIndonesianRange(cursor, sitEnd), PhaseSIT))
	cursor = sitEnd.AddDate(0, 0, 1)
	uatEnd := cursor.AddDate(0, 0, 13)
	rows = append(rows, phaseRow("User Acceptance Testing (UAT)", formatIndonesianRange(cursor, uatEnd), PhaseUAT))
	cursor = uatEnd.AddDate(0, 0, 1)
	goLiveEnd := cursor.AddDate(0, 0, 6)
	rows = append(rows, phaseRow("Go-Live", formatIndonesianRange(cursor, goLiveEnd), PhaseGoLive))
	if rules.IncludeMaintenance {
		maintenanceStart := goLiveEnd.AddDate(0, 0, 1)
		maintenanceEnd := maintenanceStart.AddDate(1, 0, -1)
		rows = append(rows, phaseRow("Support & Maintenance", formatIndonesianRange(maintenanceStart, maintenanceEnd), PhaseMaintenance))
	}
	return rows
}

func generateSprintTimeline(input domain.DedicatedSquadInput) ([]domain.TimelineRow, error) {
	if input.Timeline.Mode != domain.TimelineSprint {
		return nil, fmt.Errorf("timeline.mode: must be sprint for model 1D")
	}
	if input.Timeline.StartDate == nil {
		return nil, fmt.Errorf("timeline.start_date: is required for model 1D")
	}
	if input.Timeline.SprintEndDate == nil {
		return nil, fmt.Errorf("timeline.sprint_end_date: is required for model 1D")
	}
	start, end := calendarDate(*input.Timeline.StartDate), calendarDate(*input.Timeline.SprintEndDate)
	if end.Before(start) {
		return nil, fmt.Errorf("timeline.sprint_end_date: must not be before timeline.start_date")
	}
	tasks := requirementTasks(input.Requirement)
	if len(tasks) == 0 {
		return nil, fmt.Errorf("requirement: must contain at least one concrete sprint task")
	}
	workdays := inclusiveBusinessDays(start, end)
	unit := "Hari Kerja"
	return []domain.TimelineRow{{Description: "Sprint Task List", Timeline: fmt.Sprintf("%s (%d %s)", formatIndonesianRange(start, end), workdays, unit), Deliverables: tasks}}, nil
}

func phaseRow(description, timeline string, phase PhaseCode) domain.TimelineRow {
	return domain.TimelineRow{Description: description, Timeline: timeline, Deliverables: DeliverablesFor(phase)}
}

func developmentRow(timeline string, modules []domain.Module) domain.TimelineRow {
	deliverables := make([]string, 0, len(modules))
	for _, module := range modules {
		deliverables = append(deliverables, module.Name)
		for _, feature := range module.SubFeatures {
			deliverables = append(deliverables, "- "+feature)
		}
	}
	return domain.TimelineRow{Description: "Development", Timeline: timeline, Deliverables: deliverables}
}

func requirementTasks(requirement string) []string {
	lines := strings.Split(strings.ReplaceAll(requirement, "\r\n", "\n"), "\n")
	tasks := make([]string, 0, len(lines))
	for _, line := range lines {
		task := strings.TrimSpace(line)
		task = strings.TrimSpace(strings.TrimLeft(task, "-*•"))
		task = strings.TrimSpace(numberedTaskPrefix.ReplaceAllString(task, ""))
		if task != "" {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func inclusiveBusinessDays(start, end time.Time) int {
	count := 0
	for day := calendarDate(start); !day.After(calendarDate(end)); day = day.AddDate(0, 0, 1) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			count++
		}
	}
	return count
}

func formatIndonesianRange(start, end time.Time) string {
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	if start.Year() == end.Year() && start.Month() == end.Month() {
		return fmt.Sprintf("%d - %d %s %d", start.Day(), end.Day(), months[start.Month()], end.Year())
	}
	if start.Year() == end.Year() {
		return fmt.Sprintf("%d %s - %d %s %d", start.Day(), months[start.Month()], end.Day(), months[end.Month()], end.Year())
	}
	return fmt.Sprintf("%d %s %d - %d %s %d", start.Day(), months[start.Month()], start.Year(), end.Day(), months[end.Month()], end.Year())
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}
