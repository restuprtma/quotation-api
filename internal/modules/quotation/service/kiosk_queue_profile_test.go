package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func sectionCodes(sections []domain.DocumentSection) []string {
	codes := make([]string, len(sections))
	for i, s := range sections {
		codes[i] = s.Code
	}
	return codes
}

func TestBuildKioskQueueSectionPlan(t *testing.T) {
	tests := []struct {
		name string
		in   domain.KioskQueueSystemInput
		want []string
	}{
		{"government with hardware", governmentHardwareFixtureInput(), []string{"scope", "experts", "costs", "payment", "limitations"}},
		{"government service only", governmentServiceOnlyFixtureInput(), []string{"scope", "experts", "costs", "payment", "limitations"}},
		{"private with maintenance", privateFoodCourtFixtureInput(), []string{"development", "maintenance", "timeline", "payment", "limitations"}},
		{"private without maintenance", privateWithoutMaintenanceInput(), []string{"development", "timeline", "payment", "limitations"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := service.ResolveKioskQueueProfile(tt.in)
			require.NoError(t, err)
			sections := service.BuildKioskQueueSectionPlan(profile)
			assert.Equal(t, tt.want, sectionCodes(sections))
			for i, section := range sections {
				assert.Equal(t, i+1, section.Number)
			}
		})
	}
}

func TestResolveKioskQueueProfile_Properties(t *testing.T) {
	t.Run("government with hardware", func(t *testing.T) {
		in := governmentHardwareFixtureInput()
		in.RightsStatements = []string{"Hak cipta milik pemberi kerja."}
		profile, err := service.ResolveKioskQueueProfile(in)
		require.NoError(t, err)

		assert.Equal(t, domain.KioskQueueGovernmentMPP, profile.Variant)
		assert.Equal(t, domain.HardwareVenturoSupplied, profile.HardwareProvision)
		assert.Equal(t, domain.KioskTaxInclusivePPN11, profile.TaxTreatment)
		assert.Contains(t, profile.TaxClauseText, "PPN 11%")
		assert.True(t, profile.ShowHardware)
		assert.False(t, profile.ShowMaintenance)
		assert.True(t, profile.ShowRights)
		assert.EqualValues(t, 14, profile.PaymentDueDays)
		assert.NotEmpty(t, profile.LimitationTexts)
		assert.Equal(t, 5, len(profile.Sections))
	})

	t.Run("government service only", func(t *testing.T) {
		in := governmentServiceOnlyFixtureInput()
		profile, err := service.ResolveKioskQueueProfile(in)
		require.NoError(t, err)

		assert.Equal(t, domain.KioskQueueGovernmentMPP, profile.Variant)
		assert.Equal(t, domain.HardwareClientSupplied, profile.HardwareProvision)
		assert.Equal(t, domain.KioskTaxInclusivePPN11, profile.TaxTreatment)
		assert.Contains(t, profile.TaxClauseText, "PPN 11%")
		assert.False(t, profile.ShowHardware)
		assert.False(t, profile.ShowMaintenance)
		assert.False(t, profile.ShowRights)
		assert.EqualValues(t, 14, profile.PaymentDueDays)
		assert.NotEmpty(t, profile.LimitationTexts)
		assert.Equal(t, 5, len(profile.Sections))
	})

	t.Run("private food court with maintenance", func(t *testing.T) {
		in := privateFoodCourtFixtureInput()
		profile, err := service.ResolveKioskQueueProfile(in)
		require.NoError(t, err)

		assert.Equal(t, domain.KioskQueuePrivateFoodCourt, profile.Variant)
		assert.Equal(t, domain.HardwareClientSupplied, profile.HardwareProvision)
		assert.Equal(t, domain.KioskTaxExclusivePPN, profile.TaxTreatment)
		assert.Contains(t, profile.TaxClauseText, "belum termasuk PPN")
		assert.False(t, profile.ShowHardware)
		assert.True(t, profile.ShowMaintenance)
		assert.False(t, profile.ShowRights)
		assert.EqualValues(t, 0, profile.PaymentDueDays)
		assert.NotEmpty(t, profile.LimitationTexts)
		assert.Equal(t, 5, len(profile.Sections))
	})

	t.Run("private without maintenance", func(t *testing.T) {
		in := privateWithoutMaintenanceInput()
		profile, err := service.ResolveKioskQueueProfile(in)
		require.NoError(t, err)

		assert.False(t, profile.ShowMaintenance)
		assert.Equal(t, 4, len(profile.Sections))
		assert.Equal(t, []string{"development", "timeline", "payment", "limitations"}, sectionCodes(profile.Sections))
	})
}

func TestResolveKioskQueueProfile_LimitationCatalogCoverage(t *testing.T) {
	// Verify all 8 limitation codes have non-empty text
	allCodes := []domain.KioskQueueLimitationCode{
		domain.KioskLimitCivilElectrical,
		domain.KioskLimitClientHardware,
		domain.KioskLimitExtraHardware,
		domain.KioskLimitHardwareWarranty,
		domain.KioskLimitOutOfPocket,
		domain.KioskLimitOvertime,
		domain.KioskLimitThirdParty,
		domain.KioskLimitScopeChange,
	}

	for _, code := range allCodes {
		text, ok := service.KioskQueueLimitationCatalog[code]
		assert.True(t, ok, "limitation code %s should exist in catalog", code)
		assert.NotEmpty(t, text, "limitation text for %s should not be empty", code)
	}

	// Verify specific wording requirements
	clientHardwareText := strings.ToLower(service.KioskQueueLimitationCatalog[domain.KioskLimitClientHardware])
	assert.Contains(t, clientHardwareText, "perangkat fisik")
	assert.Contains(t, clientHardwareText, "jaringan lokasi")
	assert.Contains(t, clientHardwareText, "disediakan klien")
	assert.Contains(t, clientHardwareText, "memasang/mengonfigurasi perangkat lunak")

	civilText := strings.ToLower(service.KioskQueueLimitationCatalog[domain.KioskLimitCivilElectrical])
	assert.Contains(t, civilText, "bobok tembok")
	assert.Contains(t, civilText, "penarikan daya/kabel")
	assert.Contains(t, civilText, "stop kontak")
	assert.Contains(t, civilText, "bracket")
	assert.Contains(t, civilText, "perbaikan area")
	assert.Contains(t, civilText, "bukan tanggung jawab venturo")
}
