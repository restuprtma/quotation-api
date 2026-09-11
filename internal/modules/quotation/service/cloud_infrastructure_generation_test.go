package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func cloudGenerationInput() domain.CloudInfrastructureInput {
	return domain.CloudInfrastructureInput{
		DocumentNumber: "CTH/Authen-Cloud/0626/001", ServiceName: "Venturo Cloud",
		QuotationDate: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), TaxTreatment: domain.CloudTaxExclusive,
		PackageItems:    []domain.CloudPackageItemInput{{Category: domain.CloudPackageVPS, Name: "Virtual Machine Small", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900000}},
		PaymentSchedule: []domain.CloudPaymentRowInput{{WorkPeriod: "Bulan 1", Terms: "Pelunasan", InvoiceDateLabel: "30 Jun 2026", DueDateLabel: "14 Jul 2026", Component: domain.CloudPaymentMonthly, Nominal: 900000}},
		PaymentDueDays:  14, LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitOutOfPocket},
	}
}

func TestCloudInfrastructureGenerationLifecycleValidation(t *testing.T) {
	input := cloudGenerationInput()
	req := dto.GenerateQuotationRequest{CommercialScheme: domain.CommercialSchemeCloudInfrastructure, SolutionType: "cloud_infrastructure", Subject: "Authen Cloud", CloudInfrastructure: &input}
	require.NoError(t, ValidateGenerateInput(req))

	badRateCard := req
	badRateCard.RateCardID = uuid.New()
	require.Equal(t, ErrCodeInvalidRateCard, ValidateGenerateInput(badRateCard).(*GenerationError).Code)

	badLegacy := req
	badLegacy.BudgetMaximum = 1
	require.Equal(t, ErrCodeGenerationFailed, ValidateGenerateInput(badLegacy).(*GenerationError).Code)

	other := req
	other.TalentOutsourcing = &domain.TalentOutsourcingInput{}
	require.Equal(t, ErrCodeAllocationInvalid, ValidateGenerateInput(other).(*GenerationError).Code)
}

func TestCloudInfrastructureCommittedSnapshotsAreStrictAndOrdered(t *testing.T) {
	rows := []domain.CloudScopeRowInput{{Timeline: "2 Minggu"}, {Timeline: "3 Minggu"}}
	timeline := buildCloudTimelineSnapshot(rows)
	require.NoError(t, validateCloudTimelineSnapshot(rows, timeline))
	timeline[1].Timeline = "changed"
	require.Error(t, validateCloudTimelineSnapshot(rows, timeline))

	packages := cloudGenerationInput().PackageItems
	raw, err := json.Marshal(packages)
	require.NoError(t, err)
	var decoded []domain.CloudPackageItemInput
	require.NoError(t, unmarshalRequiredCloudSnapshot(raw, "package", &decoded))
	require.Equal(t, packages, decoded)
	require.Error(t, unmarshalRequiredCloudSnapshot([]byte(`{"unknown":true}`), "package", &decoded))
	require.Error(t, unmarshalRequiredCloudSnapshot([]byte(`[]`), "package", &decoded))
	require.Error(t, unmarshalRequiredCloudSnapshot(append(raw, []byte(` {}`)...), "package", &decoded))
}

func TestCloudInfrastructureNoScopeSnapshotsAndGenerationInput(t *testing.T) {
	// A no-scope quotation commits [] for both optional snapshots and must be
	// regenerable without treating those deliberately empty arrays as corruption.
	var scopes []domain.CloudScopeRowInput
	var timeline []cloudTimelineSnapshotItem
	require.NoError(t, unmarshalRequiredCloudSnapshot([]byte(`[]`), "scope", &scopes))
	require.NoError(t, unmarshalRequiredCloudSnapshot([]byte(`[]`), "timeline", &timeline))
	require.NoError(t, validateCloudTimelineSnapshot(scopes, timeline))

	input := cloudGenerationInput()
	req := dto.GenerateQuotationRequest{CommercialScheme: domain.CommercialSchemeCloudInfrastructure, SolutionType: "cloud_infrastructure", Subject: "Authen Cloud", CloudInfrastructure: &input}
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	decoded, err := decodeCloudInfrastructureGenerationInput(raw)
	require.NoError(t, err)
	require.Equal(t, req.CommercialScheme, decoded.CommercialScheme)

	require.Error(t, func() error {
		_, err := decodeCloudInfrastructureGenerationInput([]byte(`{"commercial_scheme":"cloud_infrastructure","solution_type":"wrong","subject":"x","cloud_infrastructure":{}}`))
		return err
	}())
	require.Error(t, func() error {
		_, err := decodeCloudInfrastructureGenerationInput(append(raw, []byte(` {}`)...))
		return err
	}())
}
