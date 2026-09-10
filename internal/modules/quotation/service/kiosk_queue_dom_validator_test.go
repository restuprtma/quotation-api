package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestValidateKioskQueueIntegrity_ValidGovernmentWithHardware(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	err = service.ValidateKioskQueueIntegrity(html, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.NoError(t, err)
}

func TestValidateKioskQueueIntegrity_ValidGovernmentServiceOnly(t *testing.T) {
	docIn := governmentServiceOnlyDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	err = service.ValidateKioskQueueIntegrity(html, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.NoError(t, err)
}

func TestValidateKioskQueueIntegrity_ValidPrivateFoodCourt(t *testing.T) {
	docIn := privateDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	err = service.ValidateKioskQueueIntegrity(html, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.NoError(t, err)
}

func TestValidateKioskQueueIntegrity_ValidPrivateWithoutMaintenance(t *testing.T) {
	docIn := privateWithoutMaintenanceDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	err = service.ValidateKioskQueueIntegrity(html, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.NoError(t, err)
}

func TestValidateKioskQueueIntegrityRejectsMissingHardwareRow(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Remove hardware_0 row
	start := strings.Index(html, `<tr data-role="hardware_0">`)
	require.True(t, start > 0)
	end := strings.Index(html[start:], `</tr>`)
	require.True(t, end > 0)
	mutated := html[:start] + html[start+end+5:]

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hardware")
}

func TestValidateKioskQueueIntegrityRejectsChangedPPN11(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Change PPN 11% tax cell
	mutated := strings.Replace(html, `data-field="service_tax_amount">Rp 4.874.100`, `data-field="service_tax_amount">Rp 4.000.000`, 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
}

func TestValidateKioskQueueIntegrityRejectsPaymentMismatch(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Mutate payment nominal
	mutated := strings.Replace(html, `data-field="payment_nominal">Rp 51.615.000`, `data-field="payment_nominal">Rp 50.000.000`, 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
}

func TestValidateKioskQueueIntegrityRejectsSectionGap(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Mutate section 2 to section 3, creating a gap
	mutated := strings.Replace(html, `data-section-number="2"><h2>2. Kebutuhan Tenaga Ahli</h2>`, `data-section-number="3"><h2>3. Kebutuhan Tenaga Ahli</h2>`, 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "section")
}

func TestValidateKioskQueueIntegrityRejectsGovernmentTableInPrivateDocument(t *testing.T) {
	docIn := privateDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Inject hardware cost table into private document
	mutated := strings.Replace(html, `<table data-role="hardware_cost_table">`, "", 1)
	mutated = strings.Replace(html, `</article>`, `<table data-role="hardware_cost_table"><tr><td>Unit</td></tr></table></article>`, 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
}

func TestValidateKioskQueueIntegrityRejectsClientHardwareWithoutClause(t *testing.T) {
	docIn := governmentServiceOnlyDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Remove client hardware clause
	clause := service.KioskQueueLimitationCatalog[domain.KioskLimitClientHardware]
	mutated := strings.Replace(html, clause, "Klausul diganti", 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_hardware")
}

func TestValidateKioskQueueIntegrityRejectsCivilExclusionRemoval(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Remove civil electrical exclusion
	clause := service.KioskQueueLimitationCatalog[domain.KioskLimitCivilElectrical]
	mutated := strings.Replace(html, clause, "Klausul sipil dihapus", 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "civil_electrical")
}

func TestValidateKioskQueueIntegrityRejectsUnexpectedOwnershipClaim(t *testing.T) {
	docIn := governmentDocumentInput()
	docIn.Input.RightsStatements = nil
	docIn.Profile.ShowRights = false
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// Inject unexpected rights block
	mutated := strings.Replace(html, `</article>`, `<div data-block-type="rights"><p>Hak kekayaan intelektual diserahkan kepada Pemda.</p></div></article>`, 1)

	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rights")
}

func TestCheckKioskQueueCommercialModified(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// 1. Unchanged document returns false
	modified, err := service.CheckKioskQueueCommercialModified(html, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.False(t, modified)

	// 2. Mutate service total
	mutated := strings.Replace(html, `data-field="service_total">Rp 48.385.000`, `data-field="service_total">Rp 50.000.000`, 1)
	modified, err = service.CheckKioskQueueCommercialModified(mutated, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, modified)

	// 3. Mutate hardware line total
	mutatedHw := strings.Replace(html, `data-field="hardware_total">Rp 51.615.000`, `data-field="hardware_total">Rp 52.000.000`, 1)
	modified, err = service.CheckKioskQueueCommercialModified(mutatedHw, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, modified)

	// 4. Mutate payment nominal
	mutatedPay := strings.Replace(html, `data-field="payment_total">Rp 100.000.000`, `data-field="payment_total">Rp 90.000.000`, 1)
	modified, err = service.CheckKioskQueueCommercialModified(mutatedPay, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, modified)

	// 5. Mutate document number
	mutatedNum := strings.Replace(html, `data-field="quotation_number">LANDA/MPP-Antrian/0726/003`, `data-field="quotation_number">LANDA/MPP-Antrian/0726/999`, 1)
	modified, err = service.CheckKioskQueueCommercialModified(mutatedNum, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, modified)

	// 6. Mutate scope deliverable text only (non-commercial)
	mutatedScope := strings.Replace(html, `Dokumen Analisis Kebutuhan`, `Dokumen Analisis Kebutuhan Lanjutan`, 1)
	modified, err = service.CheckKioskQueueCommercialModified(mutatedScope, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.False(t, modified, "Scope edit must not trigger commercial modified")
}

func TestCheckKioskQueueRightsModified(t *testing.T) {
	docIn := governmentDocumentInput()
	docIn.Input.RightsStatements = []string{"Hak guna pakai software diberikan kepada Klien."}
	docIn.Profile.ShowRights = true
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// 1. Unchanged rights
	mod, err := service.CheckKioskQueueRightsModified(html, docIn.Input.RightsStatements)
	require.NoError(t, err)
	assert.False(t, mod)

	// 2. Changed rights statement text
	mutated := strings.Replace(html, "Hak guna pakai software diberikan kepada Klien.", "Hak cipta eksklusif milik Klien sepenuhnya.", 1)
	mod, err = service.CheckKioskQueueRightsModified(mutated, docIn.Input.RightsStatements)
	require.NoError(t, err)
	assert.True(t, mod)

	// 3. Unexpected rights added when statements empty
	docNoRights := governmentDocumentInput()
	docNoRights.Input.RightsStatements = nil
	docNoRights.Profile.ShowRights = false
	htmlNoRights, err := service.NewKioskQueueHTMLRenderer().Render(docNoRights)
	require.NoError(t, err)

	mod, err = service.CheckKioskQueueRightsModified(htmlNoRights, nil)
	require.NoError(t, err)
	assert.False(t, mod)

	mutatedAdded := strings.Replace(htmlNoRights, `</article>`, `<div data-block-type="rights"><p>Hak milik</p></div></article>`, 1)
	mod, err = service.CheckKioskQueueRightsModified(mutatedAdded, nil)
	require.NoError(t, err)
	assert.True(t, mod)
}

func TestKioskQueue_ValidateAndCommercialCheck_PaymentDueClause(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// 1. Mutate due days text from H+14 to H+30
	mutated := strings.Replace(html, "H+14", "H+30", 1)
	err = service.ValidateKioskQueueIntegrity(mutated, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payment_due_clause")

	commercialMod, err := service.CheckKioskQueueCommercialModified(mutated, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, commercialMod)

	// 2. Remove payment_due_clause entirely
	mutatedRemoved := strings.Replace(html, `<p data-field="payment_due_clause">Pembayaran dilakukan maksimal H+14 hari kalender setelah invoice diterbitkan.</p>`, "", 1)
	err = service.ValidateKioskQueueIntegrity(mutatedRemoved, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payment_due_clause")

	commercialMod, err = service.CheckKioskQueueCommercialModified(mutatedRemoved, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, commercialMod)

	// 3. Unexpected payment_due_clause when PaymentDueDays == 0
	docZeroDue := privateDocumentInput()
	docZeroDue.Profile.PaymentDueDays = 0
	docZeroDue.Input.PaymentDueDays = 0
	htmlZero, err := service.NewKioskQueueHTMLRenderer().Render(docZeroDue)
	require.NoError(t, err)

	mutatedInjected := strings.Replace(htmlZero, `</section>`, `<p data-field="payment_due_clause">Pembayaran dilakukan maksimal H+7 hari kalender setelah invoice diterbitkan.</p></section>`, 1)
	err = service.ValidateKioskQueueIntegrity(mutatedInjected, docZeroDue.Input, docZeroDue.Calculation, docZeroDue.Profile, docZeroDue.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payment_due_clause")

	commercialMod, err = service.CheckKioskQueueCommercialModified(mutatedInjected, docZeroDue.Input, docZeroDue.Calculation, docZeroDue.Profile)
	require.NoError(t, err)
	assert.True(t, commercialMod)
}

func TestKioskQueue_ValidateAndCommercialCheck_RejectsSilentRowInjection(t *testing.T) {
	docIn := governmentDocumentInput()
	html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
	require.NoError(t, err)

	// 1. Injected service row with data-role
	injectedService := strings.Replace(html, `<tr data-role="service_0_0">`, `<tr data-role="service_0_99"><td>Ghost Service</td><td>Rp 10.000.000</td><td>1</td><td>1 Bulan</td><td data-field="service_line_total">Rp 10.000.000</td></tr><tr data-role="service_0_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedService, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service row count mismatch")

	commMod, err := service.CheckKioskQueueCommercialModified(injectedService, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, commMod)

	// 2. Injected hardware row
	injectedHw := strings.Replace(html, `<tr data-role="hardware_0">`, `<tr data-role="hardware_99"><td>Ghost Hardware</td><td>Rp 5.000.000</td><td>1</td><td data-field="hardware_line_total">Rp 5.000.000</td></tr><tr data-role="hardware_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedHw, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hardware row count mismatch")

	commMod, err = service.CheckKioskQueueCommercialModified(injectedHw, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, commMod)

	// 3. Injected government payment row
	injectedGovPay := strings.Replace(html, `<tr data-role="government_payment_0">`, `<tr data-role="government_payment_99"><td>Extra Stage</td><td>Basis</td><td data-field="payment_nominal">Rp 10.000.000</td></tr><tr data-role="government_payment_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedGovPay, docIn.Input, docIn.Calculation, docIn.Profile, docIn.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "government payment row count mismatch")

	commMod, err = service.CheckKioskQueueCommercialModified(injectedGovPay, docIn.Input, docIn.Calculation, docIn.Profile)
	require.NoError(t, err)
	assert.True(t, commMod)

	// 4. Injected private maintenance row in private document
	docPriv := privateDocumentInput()
	htmlPriv, err := service.NewKioskQueueHTMLRenderer().Render(docPriv)
	require.NoError(t, err)

	injectedMaint := strings.Replace(htmlPriv, `<tr data-role="maintenance_0">`, `<tr data-role="maintenance_99"><td>Ghost Squad</td><td>Rp 5.000.000</td><td>1</td><td>1 Qtr</td><td data-field="maintenance_line_total">Rp 5.000.000</td></tr><tr data-role="maintenance_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedMaint, docPriv.Input, docPriv.Calculation, docPriv.Profile, docPriv.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maintenance row count mismatch")

	commMod, err = service.CheckKioskQueueCommercialModified(injectedMaint, docPriv.Input, docPriv.Calculation, docPriv.Profile)
	require.NoError(t, err)
	assert.True(t, commMod)

	// 5. Injected private payment row in private document
	injectedPrivPay := strings.Replace(htmlPriv, `<tr data-role="private_payment_0">`, `<tr data-role="private_payment_99"><td>WP</td><td>Terms</td><td>Inv</td><td>Due</td><td data-field="payment_nominal">Rp 1.000.000</td></tr><tr data-role="private_payment_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedPrivPay, docPriv.Input, docPriv.Calculation, docPriv.Profile, docPriv.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private payment row count mismatch")

	commMod, err = service.CheckKioskQueueCommercialModified(injectedPrivPay, docPriv.Input, docPriv.Calculation, docPriv.Profile)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestKioskQueue_ValidateIntegrity_ExpertsAndTimelineRowCounts(t *testing.T) {
	// 1. Government experts injection and removal
	docGov := governmentDocumentInput()
	htmlGov, err := service.NewKioskQueueHTMLRenderer().Render(docGov)
	require.NoError(t, err)

	// Inject extra expert row
	injectedExpert := strings.Replace(htmlGov, `<tr data-role="expert_0">`, `<tr data-role="expert_1"><td>Senior DBA</td><td>1</td><td>3 Bulan</td><td>S1 Informatika</td></tr><tr data-role="expert_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedExpert, docGov.Input, docGov.Calculation, docGov.Profile, docGov.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expert row count mismatch")

	// Expert edit does NOT trigger commercial modified
	commMod, err := service.CheckKioskQueueCommercialModified(injectedExpert, docGov.Input, docGov.Calculation, docGov.Profile)
	require.NoError(t, err)
	assert.False(t, commMod, "Expert edit is non-commercial")

	// Remove expert row
	start := strings.Index(htmlGov, `<tr data-role="expert_0">`)
	require.True(t, start > 0)
	end := strings.Index(htmlGov[start:], `</tr>`)
	require.True(t, end > 0)
	removedExpert := htmlGov[:start] + htmlGov[start+end+5:]
	err = service.ValidateKioskQueueIntegrity(removedExpert, docGov.Input, docGov.Calculation, docGov.Profile, docGov.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expert")

	// 2. Private timeline injection and removal
	docPriv := privateDocumentInput()
	htmlPriv, err := service.NewKioskQueueHTMLRenderer().Render(docPriv)
	require.NoError(t, err)

	// Inject extra timeline row
	injectedTimeline := strings.Replace(htmlPriv, `<tr data-role="timeline_0">`, `<tr data-role="timeline_99"><td>Extra Phase</td><td>Week 10</td></tr><tr data-role="timeline_0">`, 1)
	err = service.ValidateKioskQueueIntegrity(injectedTimeline, docPriv.Input, docPriv.Calculation, docPriv.Profile, docPriv.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeline row count mismatch")

	// Timeline edit does NOT trigger commercial modified
	commMod, err = service.CheckKioskQueueCommercialModified(injectedTimeline, docPriv.Input, docPriv.Calculation, docPriv.Profile)
	require.NoError(t, err)
	assert.False(t, commMod, "Timeline edit is non-commercial")

	// Remove timeline row
	tStart := strings.Index(htmlPriv, `<tr data-role="timeline_0">`)
	require.True(t, tStart > 0)
	tEnd := strings.Index(htmlPriv[tStart:], `</tr>`)
	require.True(t, tEnd > 0)
	removedTimeline := htmlPriv[:tStart] + htmlPriv[tStart+tEnd+5:]
	err = service.ValidateKioskQueueIntegrity(removedTimeline, docPriv.Input, docPriv.Calculation, docPriv.Profile, docPriv.LegalHTML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeline")
}
