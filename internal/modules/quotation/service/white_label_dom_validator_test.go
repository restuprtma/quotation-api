package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func renderValidWhiteLabelCanonicalHTML(t *testing.T, input domain.WhiteLabelInput) (string, domain.WhiteLabelCalculation) {
	t.Helper()
	calc, err := service.CalculateWhiteLabel(input)
	require.NoError(t, err)

	profile, err := service.ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	renderer := service.NewWhiteLabelHTMLRenderer()
	docInput := service.WhiteLabelDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber:   "Q-2026-WL-001",
			Subject:           "Penawaran White Label Test",
			ClientName:        "Budi Santoso",
			ClientCompanyName: "PT Mitra Sejahtera",
			IssueDate:         "8 September 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Ketentuan umum berlaku.</p>",
	}
	html, err := renderer.Render(docInput)
	require.NoError(t, err)

	sanitizer := service.NewHTMLSanitizer()
	sanitized, err := sanitizer.Sanitize(html)
	require.NoError(t, err)

	return sanitized, calc
}

func ponpesWhiteLabelInput() domain.WhiteLabelInput {
	return domain.WhiteLabelInput{
		ProductName:   "POS Mini Market",
		QuotationDate: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
		TaxTreatment:  domain.WhiteLabelTaxExclusive,
		LicenseItems: []domain.WhiteLabelLicenseItemInput{
			{Name: "POS Mini Market Core", UnitRate: 35_000_000, Quantity: 1},
		},
		MaintenanceItems: []domain.WhiteLabelMaintenanceItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 16_170_000, FTEQuarters: 2, PeriodMonths: 3},
		},
		HostingItems: []domain.WhiteLabelHostingItemInput{
			{Name: "Shared Hosting Small", MonthlyRate: 350_000, Quantity: 1, PeriodMonths: 3},
		},
		ModuleGroups: []domain.WhiteLabelModuleGroupInput{
			{Name: "Inventory", Features: []string{"Stok masuk", "Stok keluar"}},
		},
		TimelineRows: []domain.WhiteLabelTimelineRowInput{
			{Description: "Setup Server & Database", Timeline: "Minggu 1"},
		},
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 2", Terms: "Bulanan", InvoiceDateLabel: "Bulan 2", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 3", Terms: "Bulanan", InvoiceDateLabel: "Bulan 3", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
		},
		RightsStatements: []string{
			"Klien berhak menggunakan source code untuk kebutuhan internal.",
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func licenseOnlyWhiteLabelInput() domain.WhiteLabelInput {
	return domain.WhiteLabelInput{
		ProductName:   "POS Mini Market Core",
		QuotationDate: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
		TaxTreatment:  domain.WhiteLabelTaxNotStated,
		LicenseItems: []domain.WhiteLabelLicenseItemInput{
			{Name: "POS Mini Market Core", UnitRate: 35_000_000, Quantity: 1},
		},
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
		},
		LimitationCodes: []string{"out_of_pocket", "overtime"},
	}
}

func TestCheckWhiteLabelCommercialModified_CanonicalMatches(t *testing.T) {
	input := ponpesWhiteLabelInput()
	html, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	commMod, err := service.CheckWhiteLabelCommercialModified(html, calc)
	require.NoError(t, err)
	assert.False(t, commMod, "Canonical HTML must not be detected as commercial modified")
}

func TestCheckWhiteLabelCommercialModified_LicenseOnlyCanonicalMatches(t *testing.T) {
	input := licenseOnlyWhiteLabelInput()
	html, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	commMod, err := service.CheckWhiteLabelCommercialModified(html, calc)
	require.NoError(t, err)
	assert.False(t, commMod, "Canonical license-only HTML must not be detected as commercial modified")
}

func TestCheckWhiteLabelCommercialModified_LicenseModifications(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	t.Run("UnitRateModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "Rp 35.000.000", "Rp 40.000.000", 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("QuantityModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td>1</td><td data-field="license_line_total"`, `<td>2</td><td data-field="license_line_total"`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("LineTotalModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="license_line_total">Rp 35.000.000</td>`, `<td data-field="license_line_total">Rp 50.000.000</td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("SubtotalModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="total_license"><strong>Rp 35.000.000</strong></td>`, `<td data-field="total_license"><strong>Rp 45.000.000</strong></td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("MissingCommercialBlock", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `data-block-type="commercial"`, `data-block-type="other"`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})
}

func TestCheckWhiteLabelCommercialModified_MaintenanceHostingModifications(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	t.Run("MaintenanceRateModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "Rp 16.170.000", "Rp 18.000.000", 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("MaintenanceFTEModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "<td>0.5</td>", "<td>1</td>", 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("MaintenancePeriodModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "<td>3 Bulan</td>", "<td>6 Bulan</td>", 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("MaintenanceLineTotalModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="maintenance_line_total">Rp 24.255.000</td>`, `<td data-field="maintenance_line_total">Rp 30.000.000</td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("HostingRateModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "Rp 350.000", "Rp 500.000", 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("HostingLineTotalModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="hosting_line_total">Rp 1.050.000</td>`, `<td data-field="hosting_line_total">Rp 2.000.000</td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("MaintenanceHostingSubtotalModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="maintenance_hosting_total"><strong>Rp 25.305.000</strong></td>`, `<td data-field="maintenance_hosting_total"><strong>Rp 30.000.000</strong></td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("UnexpectedMaintenanceHostingBlockInLicenseOnly", func(t *testing.T) {
		licInput := licenseOnlyWhiteLabelInput()
		licHTML, licCalc := renderValidWhiteLabelCanonicalHTML(t, licInput)
		// Inject commercial-maintenance-hosting block into license-only HTML
		modified := strings.Replace(licHTML, `</article>`, `<section data-block-type="commercial-maintenance-hosting"><table></table></section></article>`, 1)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, licCalc)
		require.NoError(t, err)
		assert.True(t, commMod, "Unexpected commercial-maintenance-hosting block must be detected as modified")
	})
}

func TestCheckWhiteLabelCommercialModified_PaymentModifications(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	t.Run("PaymentTotalModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="payment_total"><strong>Rp 60.305.000</strong></td>`, `<td data-field="payment_total"><strong>Rp 70.000.000</strong></td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("PaymentNominalModified_DifferentTotal", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td data-field="payment_nominal">Rp 8.435.000</td>`, `<td data-field="payment_nominal">Rp 10.000.000</td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("PaymentNominalModified_SameTotalTampering", func(t *testing.T) {
		// Compensate so total payment remains exactly Rp 60.305.000
		// Reduce License by 5.000.000 (35jt -> 30jt) and increase Maintenance 1 by 5.000.000 (8.435jt -> 13.435jt)
		modified := strings.Replace(canonicalHTML, `<td data-field="payment_nominal">Rp 35.000.000</td>`, `<td data-field="payment_nominal">Rp 30.000.000</td>`, 1)
		modified = strings.Replace(modified, `<td data-field="payment_nominal">Rp 8.435.000</td>`, `<td data-field="payment_nominal">Rp 13.435.000</td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Same-total tampering across components must be detected as modified")
	})

	t.Run("PaymentNominalModified_SameTotalWithinComponent", func(t *testing.T) {
		// Compensate within maintenance component: Maint 1 -1jt (7.435jt), Maint 2 +1jt (9.435jt)
		// Component total is unchanged, grand total unchanged, but individual rows tampered
		firstMaint := `<tr data-role="payment_1" data-component="maintenance_hosting" data-payment-index="1"><td>Maintenance 1</td><td>Bulanan</td><td>Bulan 1</td><td>H+14</td><td data-field="payment_nominal">Rp 8.435.000</td></tr>`
		firstMaintMod := `<tr data-role="payment_1" data-component="maintenance_hosting" data-payment-index="1"><td>Maintenance 1</td><td>Bulanan</td><td>Bulan 1</td><td>H+14</td><td data-field="payment_nominal">Rp 7.435.000</td></tr>`
		secondMaint := `<tr data-role="payment_2" data-component="maintenance_hosting" data-payment-index="2"><td>Maintenance 2</td><td>Bulanan</td><td>Bulan 2</td><td>H+14</td><td data-field="payment_nominal">Rp 8.435.000</td></tr>`
		secondMaintMod := `<tr data-role="payment_2" data-component="maintenance_hosting" data-payment-index="2"><td>Maintenance 2</td><td>Bulanan</td><td>Bulan 2</td><td>H+14</td><td data-field="payment_nominal">Rp 9.435.000</td></tr>`
		modified := strings.Replace(canonicalHTML, firstMaint, firstMaintMod, 1)
		modified = strings.Replace(modified, secondMaint, secondMaintMod, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Same-total tampering within component must be detected as modified")
	})

	t.Run("PaymentRows_SwappedOrder", func(t *testing.T) {
		// Swap row 0 (license) and row 1 (maintenance)
		row0 := `<tr data-role="payment_0" data-component="license" data-payment-index="0"><td>Pembelian White Label</td><td>Pelunasan</td><td>Saat kontrak ditandatangani</td><td>H+14</td><td data-field="payment_nominal">Rp 35.000.000</td></tr>`
		row1 := `<tr data-role="payment_1" data-component="maintenance_hosting" data-payment-index="1"><td>Maintenance 1</td><td>Bulanan</td><td>Bulan 1</td><td>H+14</td><td data-field="payment_nominal">Rp 8.435.000</td></tr>`
		modified := strings.Replace(canonicalHTML, row0+row1, row1+row0, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Swapped payment row order must be detected as modified")
	})

	t.Run("PaymentRows_RowRemoved", func(t *testing.T) {
		row3 := `<tr data-role="payment_3" data-component="maintenance_hosting" data-payment-index="3"><td>Maintenance 3</td><td>Bulanan</td><td>Bulan 3</td><td>H+14</td><td data-field="payment_nominal">Rp 8.435.000</td></tr>`
		modified := strings.Replace(canonicalHTML, row3, "", 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Removed payment row must be detected as modified")
	})

	t.Run("PaymentRows_RowAdded", func(t *testing.T) {
		extraRow := `<tr data-role="payment_4" data-component="maintenance_hosting" data-payment-index="4"><td>Maintenance 4</td><td>Bulanan</td><td>Bulan 4</td><td>H+14</td><td data-field="payment_nominal">Rp 8.435.000</td></tr>`
		row3 := `<tr data-role="payment_3" data-component="maintenance_hosting" data-payment-index="3"><td>Maintenance 3</td><td>Bulanan</td><td>Bulan 3</td><td>H+14</td><td data-field="payment_nominal">Rp 8.435.000</td></tr>`
		modified := strings.Replace(canonicalHTML, row3, row3+extraRow, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Added payment row must be detected as modified")
	})

	t.Run("PaymentRows_IllegalComponent", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `data-component="license"`, `data-component="arbitrary_component"`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Illegal payment component must be detected as modified")
	})

	t.Run("PaymentRows_LabelOrDueDateModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `<td>Saat kontrak ditandatangani</td>`, `<td>30 Hari Setelah Kontrak</td>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod, "Modified payment terms or due date label must be detected as modified")
	})

	t.Run("MissingPaymentBlock", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `data-block-type="payment"`, `data-block-type="other_payment"`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})
}

func TestCheckWhiteLabelRightsModified(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, _ := renderValidWhiteLabelCanonicalHTML(t, input)

	t.Run("CanonicalRightsMatches", func(t *testing.T) {
		rightsMod, err := service.CheckWhiteLabelRightsModified(canonicalHTML, input.RightsStatements)
		require.NoError(t, err)
		assert.False(t, rightsMod, "Canonical rights must not be detected as modified")
	})

	t.Run("RightsTextModified", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "Klien berhak menggunakan source code", "Klien memiliki hak milik penuh atas source code", 1)
		require.NotEqual(t, canonicalHTML, modified)
		rightsMod, err := service.CheckWhiteLabelRightsModified(modified, input.RightsStatements)
		require.NoError(t, err)
		assert.True(t, rightsMod)
	})

	t.Run("RightsRemovedWhenExpected", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, `data-block-type="rights"`, `data-block-type="other"`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		rightsMod, err := service.CheckWhiteLabelRightsModified(modified, input.RightsStatements)
		require.NoError(t, err)
		assert.True(t, rightsMod)
	})

	t.Run("RightsAddedWhenEmptyExpected", func(t *testing.T) {
		licInput := licenseOnlyWhiteLabelInput()
		licHTML, _ := renderValidWhiteLabelCanonicalHTML(t, licInput)

		// Assert license-only without rights is not modified
		rightsMod, err := service.CheckWhiteLabelRightsModified(licHTML, nil)
		require.NoError(t, err)
		assert.False(t, rightsMod)

		// Inject rights block
		withRights := strings.Replace(licHTML, `</article>`, `<div data-block-type="rights"><p>Hak cipta diserahkan.</p></div></article>`, 1)
		rightsMod, err = service.CheckWhiteLabelRightsModified(withRights, nil)
		require.NoError(t, err)
		assert.True(t, rightsMod, "Adding rights when none were expected must be detected as modified")
	})
}

func TestCheckWhiteLabel_NarrativeEditDoesNotTriggerCommercialOrLegal(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	// Modify narrative in opening paragraph, module group heading, feature text, timeline description
	modified := strings.Replace(canonicalHTML, "Melalui penawaran ini", "Melalui penawaran resmi ini", 1)
	modified = strings.Replace(modified, "<h3>Inventory</h3>", "<h3>Modul Inventori & Pergudangan</h3>", 1)
	modified = strings.Replace(modified, "<li>Stok masuk</li>", "<li>Stok barang masuk gudang</li>", 1)
	modified = strings.Replace(modified, "<td>Setup Server & Database</td>", "<td>Konfigurasi Infrastruktur & Server</td>", 1)
	require.NotEqual(t, canonicalHTML, modified)

	commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
	require.NoError(t, err)
	assert.False(t, commMod, "Narrative edits must not trigger commercial modified")

	rightsMod, err := service.CheckWhiteLabelRightsModified(modified, input.RightsStatements)
	require.NoError(t, err)
	assert.False(t, rightsMod, "Narrative edits must not trigger rights modified")
}

func TestCheckWhiteLabel_DuplicateMarkersReturnError(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	// Duplicate commercial block
	duplicatedCommercial := canonicalHTML + `<section data-block-type="commercial"></section>`
	_, err := service.CheckWhiteLabelCommercialModified(duplicatedCommercial, calc)
	require.Error(t, err, "Duplicate commercial block must return error")

	// Duplicate rights block
	duplicatedRights := canonicalHTML + `<div data-block-type="rights"></div>`
	_, err = service.CheckWhiteLabelRightsModified(duplicatedRights, input.RightsStatements)
	require.Error(t, err, "Duplicate rights block must return error")
}

func TestCheckWhiteLabel_RowCountsAndItemNames(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	t.Run("LicenseNameMismatch", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "POS Mini Market Core", "POS Mega Market", 1)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("MaintenanceNameMismatch", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "Senior Web Programmer", "Junior Programmer", 1)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("HostingNameMismatch", func(t *testing.T) {
		modified := strings.Replace(canonicalHTML, "Shared Hosting Small", "Dedicated Server High", 1)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})

	t.Run("ExtraLicenseRow", func(t *testing.T) {
		extraRow := `<tr data-role="license_1"><td>Extra</td><td>Rp 10.000.000</td><td>1</td><td data-field="license_line_total">Rp 10.000.000</td></tr>`
		modified := strings.Replace(canonicalHTML, `</tr><tr><td colspan="3"><strong>TOTAL INVESTMENT</strong>`, `</tr>`+extraRow+`<tr><td colspan="3"><strong>TOTAL INVESTMENT</strong>`, 1)
		require.NotEqual(t, canonicalHTML, modified)
		commMod, err := service.CheckWhiteLabelCommercialModified(modified, calc)
		require.NoError(t, err)
		assert.True(t, commMod)
	})
}

func TestCheckWhiteLabel_SanitizationStripsMaliciousScript(t *testing.T) {
	input := ponpesWhiteLabelInput()
	canonicalHTML, calc := renderValidWhiteLabelCanonicalHTML(t, input)

	maliciousHTML := `<script>alert("hacked")</script>` + canonicalHTML
	sanitizer := service.NewHTMLSanitizer()
	sanitized, err := sanitizer.Sanitize(maliciousHTML)
	require.NoError(t, err)
	assert.NotContains(t, sanitized, "<script>")

	commMod, err := service.CheckWhiteLabelCommercialModified(sanitized, calc)
	require.NoError(t, err)
	assert.False(t, commMod)

	rightsMod, err := service.CheckWhiteLabelRightsModified(sanitized, input.RightsStatements)
	require.NoError(t, err)
	assert.False(t, rightsMod)
}

