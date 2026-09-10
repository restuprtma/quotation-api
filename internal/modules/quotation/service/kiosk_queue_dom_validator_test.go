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
