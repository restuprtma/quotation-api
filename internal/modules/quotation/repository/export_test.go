package repository

import "venturo-skeleton-go/internal/modules/quotation/domain"

func ExportScanQuotationRow(scanner rowScanner) (*domain.Quotation, error) {
	return scanQuotationRow(scanner)
}
