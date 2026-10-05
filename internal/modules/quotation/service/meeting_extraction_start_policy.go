package service

import (
	"context"
	"sync/atomic"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

// MeetingExtractionStartDecider makes the provider capability decision before
// a pipeline run is created. Legacy fallback is represented by a safe counter.
type MeetingExtractionStartDecider struct {
	providers       ProviderSource
	mode            string
	samplingPercent int
	random          func() int
	legacyFallbacks atomic.Uint64
}

func NewMeetingExtractionStartDecider(providers ProviderSource, mode string, samplingPercent int) *MeetingExtractionStartDecider {
	if samplingPercent < 0 {
		samplingPercent = 0
	}
	if samplingPercent > 100 {
		samplingPercent = 100
	}
	return &MeetingExtractionStartDecider{providers: providers, mode: mode, samplingPercent: samplingPercent}
}

func (d *MeetingExtractionStartDecider) Decide(ctx context.Context, companyID uuid.UUID, req dto.StartMeetingExtractionRequest) (bool, error) {
	if d == nil || d.mode == "legacy" {
		return false, nil
	}
	if req.ServiceScheme != "" && !PipelineSchemeReady(req.ServiceScheme) {
		d.legacyFallbacks.Add(1)
		return false, nil
	}
	if d.providers == nil {
		return false, ErrMeetingExtractionExecutor
	}
	provider, _, _, err := d.providers.ProviderForCompany(ctx, companyID)
	if err != nil {
		d.legacyFallbacks.Add(1)
		return false, nil
	}
	if provider == nil {
		d.legacyFallbacks.Add(1)
		return false, nil
	}
	if _, ok := provider.(ExtractionProvider); !ok {
		d.legacyFallbacks.Add(1)
		return false, nil
	}
	if d.mode == "shadow" {
		if d.samplingPercent == 0 {
			return false, nil
		}
		if d.random != nil && d.random()%100 >= d.samplingPercent {
			return false, nil
		}
	}
	return true, nil
}

func (d *MeetingExtractionStartDecider) LegacyFallbacks() uint64 {
	if d == nil {
		return 0
	}
	return d.legacyFallbacks.Load()
}
