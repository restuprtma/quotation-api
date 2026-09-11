package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type cloudInfrastructureBenchmarkFixture struct {
	SchemaVersion string                          `json:"schema_version"`
	Metadata      benchmarkMetadata               `json:"metadata"`
	Input         domain.CloudInfrastructureInput `json:"input"`
}

func TestCloudInfrastructureBenchmarkAuthen(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(sourceFile), "testdata", "cloud_infrastructure", "authen.json")
	fixture := loadCloudInfrastructureBenchmarkFixture(t, path)
	require.Equal(t, "cloud-infrastructure-benchmark-v1", fixture.SchemaVersion)
	require.Equal(t, "historical_sheet_reference_repaired_payment", fixture.Metadata.SourceStatus)
	require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
	require.NotEmpty(t, fixture.Metadata.SourceVariance)

	assertCloudInfrastructureBenchmarkOutput(t, fixture.Input, fixture.Metadata, 1_800_000, true)
}

func TestCloudInfrastructureBenchmarkSynthesizedContractCoverage(t *testing.T) {
	t.Run("shared hosting without scope", func(t *testing.T) {
		in := domain.CloudInfrastructureInput{
			DocumentNumber: "SYN/Shared/001", ServiceName: "Venturo Cloud", QuotationDate: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
			TaxTreatment:    domain.CloudTaxNotStated,
			PackageItems:    []domain.CloudPackageItemInput{{Category: domain.CloudPackageSharedHosting, Name: "Shared Hosting Mini", VCPU: 1, RAMGB: 2, StorageGB: 5, UnitRate: 200_000}},
			PaymentSchedule: []domain.CloudPaymentRowInput{{WorkPeriod: "Bulan 1", Terms: "Bulanan", InvoiceDateLabel: "11 Sep 2026", DueDateLabel: "25 Sep 2026", Component: domain.CloudPaymentMonthly, Nominal: 200_000}},
			PaymentDueDays:  14, LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitScopeChange},
		}
		assertCloudInfrastructureBenchmarkOutput(t, in, benchmarkMetadata{QuotationNumber: in.DocumentNumber, ClientName: "Synthetic"}, 200_000, false)
	})

	t.Run("vps with setup fee", func(t *testing.T) {
		in := domain.CloudInfrastructureInput{
			DocumentNumber: "SYN/VPS/001", ServiceName: "Venturo Cloud", QuotationDate: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
			TaxTreatment: domain.CloudTaxInclusive,
			PackageItems: []domain.CloudPackageItemInput{{Category: domain.CloudPackageVPS, Name: "Cloud VPS Small", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900_000}},
			SetupItems:   []domain.CloudSetupItemInput{{Name: "Initial server provisioning", Amount: 300_000}},
			PaymentSchedule: []domain.CloudPaymentRowInput{
				{WorkPeriod: "Setup", Terms: "Sekali", InvoiceDateLabel: "11 Sep 2026", DueDateLabel: "11 Sep 2026", Component: domain.CloudPaymentSetup, Nominal: 300_000},
				{WorkPeriod: "Bulan 1", Terms: "Bulanan", InvoiceDateLabel: "11 Sep 2026", DueDateLabel: "25 Sep 2026", Component: domain.CloudPaymentMonthly, Nominal: 900_000},
			},
			PaymentDueDays: 14, LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitOutOfPocket},
		}
		calc, err := CalculateCloudInfrastructure(in)
		require.NoError(t, err)
		require.Equal(t, int64(300_000), calc.SetupSubtotal)
		require.Equal(t, int64(300_000), calc.SetupPaymentTotal)
		require.Equal(t, int64(900_000), calc.MonthlySubtotal)
	})
}

func assertCloudInfrastructureBenchmarkOutput(t *testing.T, in domain.CloudInfrastructureInput, metadata benchmarkMetadata, expectedMonthly int64, expectScope bool) {
	t.Helper()
	in = in.Normalize()
	require.NoError(t, in.Validate())
	calc, err := CalculateCloudInfrastructure(in)
	require.NoError(t, err)
	assert.Equal(t, expectedMonthly, calc.MonthlySubtotal)
	assert.Equal(t, int32(1), calc.MonthlyPaymentRows)
	assert.Equal(t, expectedMonthly, calc.ScheduledRecurringTotal)

	profile, err := ResolveCloudInfrastructureProfile(in)
	require.NoError(t, err)
	wantSections := []string{"cloud_cost"}
	if expectScope {
		wantSections = append(wantSections, "scope_timeline")
	}
	wantSections = append(wantSections, "payment", "limitations")
	gotSections := make([]string, len(profile.Sections))
	for i, section := range profile.Sections {
		gotSections[i] = section.Code
	}
	assert.Equal(t, wantSections, gotSections)

	doc, err := NewCloudInfrastructureRenderer().Render(CloudInfrastructureDocumentInput{
		Metadata: QuotationMetadata{QuotationNumber: metadata.QuotationNumber, Subject: metadata.Subject, ClientName: metadata.ClientName, ClientCompanyName: metadata.ClientCompany, IssueDate: metadata.IssueDate},
		Input:    in, Calculation: calc, Profile: profile,
	})
	require.NoError(t, err)
	sanitized, err := NewHTMLSanitizer().Sanitize(doc)
	require.NoError(t, err)
	assert.Contains(t, sanitized, "quotation-document--cloud-infrastructure")
	assert.NotContains(t, sanitized, "#REF!")
	assert.NotContains(t, sanitized, "99,5%")
	if expectScope {
		assert.Contains(t, sanitized, "Performance / Load Test")
		assert.Contains(t, sanitized, "Penetration Test")
	}
	require.NoError(t, ValidateCloudInfrastructureIntegrity(sanitized, CloudInfrastructureIntegrityExpectations{Input: in, Calculation: calc, Profile: profile}))
}

func loadCloudInfrastructureBenchmarkFixture(t *testing.T, path string) cloudInfrastructureBenchmarkFixture {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture cloudInfrastructureBenchmarkFixture
	require.NoError(t, json.Unmarshal(b, &fixture))
	return fixture
}

func TestCloudInfrastructureBenchmarkSourceEvidenceIsExplicit(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "testdata", "cloud_infrastructure", "authen.json"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "#REF!")
	assert.True(t, strings.Contains(string(b), "replacement"))
}
