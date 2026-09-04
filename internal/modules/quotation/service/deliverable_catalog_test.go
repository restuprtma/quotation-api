package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliverableCatalog(t *testing.T) {
	tests := []struct {
		name     string
		phase    PhaseCode
		expected []string
	}{
		{"requirement", PhaseRequirement, []string{"Kick-off meeting dan identifikasi kebutuhan bisnis", "Review proses bisnis dan alur kerja yang ada", "Analisis gap antara proses bisnis saat ini dan sistem yang diinginkan", "Penyusunan dokumen Project Requirement Document (PRD)", "Wireframing dan mockup UI/UX untuk gambaran awal sistem", "Persetujuan requirement bersama klien", "Menyusun desain arsitektur sistem dan database"}},
		{"sit", PhaseSIT, []string{"End-to-End Business Process Testing (Pengujian sesuai Test Scenario)", "Error Handling & Negative Testing", "Regression testing (untuk memastikan perubahan tidak merusak fitur lain)", "Compatibility testing (uji di berbagai perangkat/browser bila relevan)"}},
		{"uat", PhaseUAT, []string{"Menjadwalkan sesi review bersama tim dan klien", "Dokumentasi hasil uji (defect log, feedback list)", "Memastikan seluruh sprint planning yang telah disepakati bersama telah dikerjakan", "Penanganan apabila muncul bug saat review dan melakukan retesting", "Memberikan training kepada klien agar sistem dapat digunakan dengan optimal", "Melakukan penambahan atau improvement sesuai hasil UAT yang disepakati", "Memverifikasi Quality Release Checklist sebelum sistem diakses pengguna", "Klien memberikan approval formal (sign-off) sebelum go-live", "Melakukan security assessment dasar (uji akses, SQL injection, dan celah umum)"}},
		{"k6", PhaseK6, []string{"Menyiapkan staging environment yang mereplikasi kondisi produksi", "Membuat script testing menggunakan tools K6", "Melakukan uji beban (Load Test) dengan jumlah pengguna bertahap (baseline → stress → endurance)", "Mencatat metrik performa (response time P95/P99, throughput, CPU, RAM, DB latency, error rate)", "Analisis hasil performa dan identifikasi bottleneck (server, database, API, query, caching, dll)", "Melakukan tuning dan optimasi sistem (Redis untuk caching, RabbitMQ untuk antrian proses)"}},
		{"pentest", PhasePentest, []string{"Menentukan Scope of Work dan Rules of Engagement (daftar domain, IP, API, batasan)", "Melakukan Reconnaissance & Information Gathering (identifikasi endpoint, versi software, dependency)", "Melakukan Vulnerability Assessment (OWASP Top 10, CVE scan, dependency scan)", "Melakukan Manual Penetration Test untuk eksploitasi terkontrol pada kerentanan potensial", "Uji autentikasi, otorisasi, logika bisnis, privilege escalation, dan validasi input", "Melakukan Post-Exploitation Analysis (menilai dampak dan risiko)", "Menyusun laporan hasil pengujian lengkap dengan severity level dan rekomendasi perbaikan"}},
		{"go-live", PhaseGoLive, []string{"Deployment checklist (server, domain, SSL, konfigurasi)", "Memantau performa dan stabilitas sistem setelah aktif", "Menyediakan hypercare support pada masa awal penggunaan", "Menangani bug, penambahan, atau improvement pasca go-live", "Melakukan security assessment dasar (uji akses, SQL injection, dan celah umum)"}},
		{"maintenance", PhaseMaintenance, []string{"Memantau performa sistem secara rutin untuk memastikan stabilitas", "Menangani bug atau error yang muncul setelah sistem berjalan", "Melakukan patching dan update keamanan secara berkala", "Menyediakan support helpdesk untuk pertanyaan dan kendala pengguna", "Melakukan preventive maintenance untuk mencegah gangguan di masa depan", "Melaksanakan backup data secara terjadwal dan memastikan restore dapat dilakukan", "Menangani request penambahan fitur atau improvement minor sesuai kebutuhan klien", "Menyusun laporan periodik terkait status sistem, bug, dan aktivitas maintenance"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeliverablesFor(tt.phase)
			assert.Equal(t, tt.expected, got)
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
