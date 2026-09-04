package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestTimelineGeneratorRelativeSnapshot(t *testing.T) {
	input := domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}
	rules := DedicatedSquadRules{IncludeTimeline: true, IncludeMaintenance: true, IncludeK6: true, IncludePentest: true}
	modules := []domain.Module{{Name: "Manajemen Pengguna", SubFeatures: []string{"Login", "Role-based access"}}}

	got, err := GenerateTimeline(input, rules, modules)
	require.NoError(t, err)
	assert.Equal(t, []string{"Requirement Analysis & System Design", "Development", "Performance / Load Test", "Penetration Test", "System Integration Testing (SIT)", "User Acceptance Testing (UAT)", "Go-Live", "Support & Maintenance"}, timelineDescriptions(got))
	assert.Equal(t, []string{"Bulan 1", "Bulan 2, 3", "2 Minggu", "2 Minggu", "Bulan 4", "2 Minggu", "Bulan 5", "Bulan 6 - 17"}, timelineLabels(got))
	assert.Equal(t, []string{"Manajemen Pengguna", "- Login", "- Role-based access"}, got[1].Deliverables)
}

func TestTimelineGeneratorCalendarSnapshot(t *testing.T) {
	start := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	input := domain.DedicatedSquadInput{Model: domain.ModelSOWSmall, Timeline: domain.TimelineInput{Mode: domain.TimelineCalendar, StartDate: &start}}
	rules := DedicatedSquadRules{IncludeTimeline: true}

	got, err := GenerateTimeline(input, rules, []domain.Module{{Name: "Dashboard", SubFeatures: []string{"Ringkasan transaksi"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"1 - 14 Juli 2026",
		"15 Juli - 8 September 2026",
		"9 - 22 September 2026",
		"23 September - 6 Oktober 2026",
		"7 - 13 Oktober 2026",
	}, timelineLabels(got))
}

func TestTimelineGeneratorConditionalRows(t *testing.T) {
	input := domain.DedicatedSquadInput{Model: domain.ModelSOWSmall, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}
	modules := []domain.Module{{Name: "Dashboard", SubFeatures: []string{"Ringkasan"}}}

	withoutSecurity, err := GenerateTimeline(input, DedicatedSquadRules{IncludeTimeline: true}, modules)
	require.NoError(t, err)
	assert.NotContains(t, timelineDescriptions(withoutSecurity), "Performance / Load Test")
	assert.NotContains(t, timelineDescriptions(withoutSecurity), "Penetration Test")
	assert.NotContains(t, timelineDescriptions(withoutSecurity), "Support & Maintenance")

	withSecurity, err := GenerateTimeline(input, DedicatedSquadRules{IncludeTimeline: true, IncludeK6: true, IncludePentest: true}, modules)
	require.NoError(t, err)
	assert.Contains(t, timelineDescriptions(withSecurity), "Performance / Load Test")
	assert.Contains(t, timelineDescriptions(withSecurity), "Penetration Test")
}

func TestTimelineGeneratorStaffAugmentationIsEmpty(t *testing.T) {
	got, err := GenerateTimeline(domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation}, DedicatedSquadRules{}, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTimelineGeneratorSprintUsesRequirementOnly(t *testing.T) {
	start := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	input := domain.DedicatedSquadInput{Model: domain.ModelSprint, Requirement: "- Tambahkan filter kategori\n- Perbaiki ekspor laporan", Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}

	got, err := GenerateTimeline(input, DedicatedSquadRules{IncludeTimeline: true, IncludeK6: true, IncludePentest: true, IncludeMaintenance: true}, []domain.Module{{Name: "Harus diabaikan", SubFeatures: []string{"AI output"}}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Sprint Task List", got[0].Description)
	assert.Equal(t, "14 - 17 Juli 2026 (4 Hari Kerja)", got[0].Timeline)
	assert.Equal(t, []string{"Tambahkan filter kategori", "Perbaiki ekspor laporan"}, got[0].Deliverables)
	assert.NotContains(t, got[0].Deliverables, "AI output")
}

func TestTimelineGeneratorSprintCountsWeekdaysInclusively(t *testing.T) {
	start := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	input := domain.DedicatedSquadInput{Model: domain.ModelSprint, Requirement: "Perbaiki login", Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}
	got, err := GenerateTimeline(input, DedicatedSquadRules{IncludeTimeline: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, "17 - 20 Juli 2026 (2 Hari Kerja)", got[0].Timeline)
}

func timelineDescriptions(rows []domain.TimelineRow) []string {
	values := make([]string, len(rows))
	for i, row := range rows {
		values[i] = row.Description
	}
	return values
}

func timelineLabels(rows []domain.TimelineRow) []string {
	values := make([]string, len(rows))
	for i, row := range rows {
		values[i] = row.Timeline
	}
	return values
}
