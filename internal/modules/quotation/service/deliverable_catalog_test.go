package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliverableCatalog(t *testing.T) {
	tests := []struct {
		name  string
		phase PhaseCode
		count int
		first string
		last  string
	}{
		{"requirement", PhaseRequirement, 7, "Kick-off meeting dan identifikasi kebutuhan bisnis", "Menyusun desain arsitektur sistem dan database"},
		{"sit", PhaseSIT, 4, "End-to-End Business Process Testing (Pengujian sesuai Test Scenario)", "Compatibility testing (uji di berbagai perangkat/browser bila relevan)"},
		{"uat", PhaseUAT, 9, "Menjadwalkan sesi review bersama tim dan klien", "Melakukan security assessment dasar (uji akses, SQL injection, dan celah umum)"},
		{"k6", PhaseK6, 6, "Menyiapkan staging environment yang mereplikasi kondisi produksi", "Melakukan tuning dan optimasi sistem (Redis untuk caching, RabbitMQ untuk antrian proses)"},
		{"pentest", PhasePentest, 7, "Menentukan Scope of Work dan Rules of Engagement (daftar domain, IP, API, batasan)", "Menyusun laporan hasil pengujian lengkap dengan severity level dan rekomendasi perbaikan"},
		{"go-live", PhaseGoLive, 5, "Deployment checklist (server, domain, SSL, konfigurasi)", "Melakukan security assessment dasar (uji akses, SQL injection, dan celah umum)"},
		{"maintenance", PhaseMaintenance, 8, "Memantau performa sistem secara rutin untuk memastikan stabilitas", "Menyusun laporan periodik terkait status sistem, bug, dan aktivitas maintenance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeliverablesFor(tt.phase)
			require.Len(t, got, tt.count)
			assert.Equal(t, tt.first, got[0])
			assert.Equal(t, tt.last, got[len(got)-1])
		})
	}
}

func TestDeliverablesForReturnsDefensiveCopy(t *testing.T) {
	first := DeliverablesFor(PhaseRequirement)
	require.NotEmpty(t, first)
	first[0] = "changed by caller"

	second := DeliverablesFor(PhaseRequirement)
	assert.Equal(t, "Kick-off meeting dan identifikasi kebutuhan bisnis", second[0])
}

func TestDeliverablesForUnknownPhase(t *testing.T) {
	assert.Empty(t, DeliverablesFor(PhaseCode("unknown")))
}
