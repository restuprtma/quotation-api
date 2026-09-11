package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestCloudInfrastructureProfile(t *testing.T) {
	t.Run("resolves Authen profile", func(t *testing.T) {
		profile, err := ResolveCloudInfrastructureProfile(validCloudInfrastructureInput().Normalize())
		require.NoError(t, err)
		assert.False(t, profile.ShowSetup)
		assert.True(t, profile.ShowScope)
		assert.Equal(t, "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.", profile.TaxClauseText)
		assert.Equal(t, []domain.DocumentSection{
			{Number: 1, Code: "cloud_cost", Title: "Biaya Venturo Cloud"},
			{Number: 2, Code: "scope_timeline", Title: "Lingkup Pekerjaan & Timeline"},
			{Number: 3, Code: "payment", Title: "Pembayaran"},
			{Number: 4, Code: "limitations", Title: "Batasan dan Pengecualian"},
		}, profile.Sections)
	})

	t.Run("omits scope and compacts section numbers", func(t *testing.T) {
		in := validCloudInfrastructureInput()
		in.ScopeRows = nil
		profile, err := ResolveCloudInfrastructureProfile(in.Normalize())
		require.NoError(t, err)
		assert.False(t, profile.ShowScope)
		assert.Equal(t, []domain.DocumentSection{{Number: 1, Code: "cloud_cost", Title: "Biaya Venturo Cloud"}, {Number: 2, Code: "payment", Title: "Pembayaran"}, {Number: 3, Code: "limitations", Title: "Batasan dan Pengecualian"}}, profile.Sections)
	})

	t.Run("shows setup without adding a section", func(t *testing.T) {
		in := validCloudInfrastructureInput()
		in.SetupItems = []domain.CloudSetupItemInput{{Name: "Setup", Amount: 500_000}}
		in.PaymentSchedule = append([]domain.CloudPaymentRowInput{{WorkPeriod: "Setup", Terms: "Pelunasan", InvoiceDateLabel: "24 Jun", DueDateLabel: "8 Jul", Component: domain.CloudPaymentSetup, Nominal: 500_000}}, in.PaymentSchedule...)
		profile, err := ResolveCloudInfrastructureProfile(in.Normalize())
		require.NoError(t, err)
		assert.True(t, profile.ShowSetup)
		assert.Len(t, profile.Sections, 4)
	})

	for _, tt := range []struct {
		name string
		tax  domain.CloudTaxTreatment
		want string
	}{
		{"inclusive", domain.CloudTaxInclusive, "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini."},
		{"not stated", domain.CloudTaxNotStated, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := validCloudInfrastructureInput()
			in.TaxTreatment = tt.tax
			profile, err := ResolveCloudInfrastructureProfile(in.Normalize())
			require.NoError(t, err)
			assert.Equal(t, tt.want, profile.TaxClauseText)
		})
	}
}
