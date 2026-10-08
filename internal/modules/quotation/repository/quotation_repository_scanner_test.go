package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type dummyScanner struct {
	values []any
}

func (s *dummyScanner) Scan(dest ...any) error {
	for i, d := range dest {
		switch target := d.(type) {
		case *uuid.UUID:
			*target = s.values[i].(uuid.UUID)
		case **uuid.UUID:
			if s.values[i] != nil {
				val := s.values[i].(uuid.UUID)
				*target = &val
			}
		case *string:
			*target = s.values[i].(string)
		case **string:
			if s.values[i] != nil {
				val := s.values[i].(string)
				*target = &val
			}
		case *int:
			*target = s.values[i].(int)
		case **int64:
			if s.values[i] != nil {
				val := s.values[i].(int64)
				*target = &val
			}
		case *time.Time:
			*target = s.values[i].(time.Time)
		case **time.Time:
			if s.values[i] != nil {
				val := s.values[i].(time.Time)
				*target = &val
			}
		}
	}
	return nil
}

func TestScanQuotationRow_MapsAllFieldsCorrectly(t *testing.T) {
	now := time.Now()
	clientName := "PT Venturo"
	picName := "Budi Santoso"
	budget := int64(50000000)
	totalInv := int64(45000000)
	qID := uuid.New()
	compID := uuid.New()
	clientID := uuid.New()

	scanner := &dummyScanner{
		values: []any{
			qID, compID, clientID, "QUO-001", "Subject A",
			"custom_software", "dedicated_squad", "draft", 1,
			uuid.New(), nil, nil,
			now, now, nil,
			clientName,
			budget,
			totalInv,
			picName,
		},
	}

	var q *domain.Quotation
	var err error
	q, err = repository.ExportScanQuotationRow(scanner)
	require.NoError(t, err)
	assert.Equal(t, qID, q.ID)
	assert.Equal(t, &clientName, q.ClientName)
	assert.Equal(t, &picName, q.PICName)
	assert.Equal(t, &totalInv, q.TotalInvestment)
	assert.Equal(t, &budget, q.BudgetMaximum)
}
