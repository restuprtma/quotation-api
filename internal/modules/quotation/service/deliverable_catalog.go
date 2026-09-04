package service

// PhaseCode identifies a canonical non-development SDLC phase.
type PhaseCode string

const (
	PhaseRequirement PhaseCode = "requirement"
	PhaseSIT         PhaseCode = "sit"
	PhaseUAT         PhaseCode = "uat"
	PhaseK6          PhaseCode = "k6"
	PhasePentest     PhaseCode = "pentest"
	PhaseGoLive      PhaseCode = "go_live"
	PhaseMaintenance PhaseCode = "maintenance"
)

var requirementDeliverables = [...]string{
	"Kick-off meeting dan identifikasi kebutuhan bisnis",
	"Review proses bisnis dan alur kerja yang ada",
	"Analisis gap antara proses bisnis saat ini dan sistem yang diinginkan",
	"Penyusunan dokumen Project Requirement Document (PRD)",
	"Wireframing dan mockup UI/UX untuk gambaran awal sistem",
	"Persetujuan requirement bersama klien",
	"Menyusun desain arsitektur sistem dan database",
}

var sitDeliverables = [...]string{
	"End-to-End Business Process Testing (Pengujian sesuai Test Scenario)",
	"Error Handling & Negative Testing",
	"Regression testing (untuk memastikan perubahan tidak merusak fitur lain)",
	"Compatibility testing (uji di berbagai perangkat/browser bila relevan)",
}

var uatDeliverables = [...]string{
	"Menjadwalkan sesi review bersama tim dan klien",
	"Dokumentasi hasil uji (defect log, feedback list)",
	"Memastikan seluruh sprint planning yang telah disepakati bersama telah dikerjakan",
	"Penanganan apabila muncul bug saat review dan melakukan retesting",
	"Memberikan training kepada klien agar sistem dapat digunakan dengan optimal",
	"Melakukan penambahan atau improvement sesuai hasil UAT yang disepakati",
	"Memverifikasi Quality Release Checklist sebelum sistem diakses pengguna",
	"Klien memberikan approval formal (sign-off) sebelum go-live",
	"Melakukan security assessment dasar (uji akses, SQL injection, dan celah umum)",
}

var k6Deliverables = [...]string{
	"Menyiapkan staging environment yang mereplikasi kondisi produksi",
	"Membuat script testing menggunakan tools K6",
	"Melakukan uji beban (Load Test) dengan jumlah pengguna bertahap (baseline → stress → endurance)",
	"Mencatat metrik performa (response time P95/P99, throughput, CPU, RAM, DB latency, error rate)",
	"Analisis hasil performa dan identifikasi bottleneck (server, database, API, query, caching, dll)",
	"Melakukan tuning dan optimasi sistem (Redis untuk caching, RabbitMQ untuk antrian proses)",
}

var pentestDeliverables = [...]string{
	"Menentukan Scope of Work dan Rules of Engagement (daftar domain, IP, API, batasan)",
	"Melakukan Reconnaissance & Information Gathering (identifikasi endpoint, versi software, dependency)",
	"Melakukan Vulnerability Assessment (OWASP Top 10, CVE scan, dependency scan)",
	"Melakukan Manual Penetration Test untuk eksploitasi terkontrol pada kerentanan potensial",
	"Uji autentikasi, otorisasi, logika bisnis, privilege escalation, dan validasi input",
	"Melakukan Post-Exploitation Analysis (menilai dampak dan risiko)",
	"Menyusun laporan hasil pengujian lengkap dengan severity level dan rekomendasi perbaikan",
}

var goLiveDeliverables = [...]string{
	"Deployment checklist (server, domain, SSL, konfigurasi)",
	"Memantau performa dan stabilitas sistem setelah aktif",
	"Menyediakan hypercare support pada masa awal penggunaan",
	"Menangani bug, penambahan, atau improvement pasca go-live",
	"Melakukan security assessment dasar (uji akses, SQL injection, dan celah umum)",
}

var maintenanceDeliverables = [...]string{
	"Memantau performa sistem secara rutin untuk memastikan stabilitas",
	"Menangani bug atau error yang muncul setelah sistem berjalan",
	"Melakukan patching dan update keamanan secara berkala",
	"Menyediakan support helpdesk untuk pertanyaan dan kendala pengguna",
	"Melakukan preventive maintenance untuk mencegah gangguan di masa depan",
	"Melaksanakan backup data secara terjadwal dan memastikan restore dapat dilakukan",
	"Menangani request penambahan fitur atau improvement minor sesuai kebutuhan klien",
	"Menyusun laporan periodik terkait status sistem, bug, dan aktivitas maintenance",
}

// DeliverablesFor returns a caller-owned copy of the canonical deliverables for phase.
func DeliverablesFor(phase PhaseCode) []string {
	var source []string
	switch phase {
	case PhaseRequirement:
		source = requirementDeliverables[:]
	case PhaseSIT:
		source = sitDeliverables[:]
	case PhaseUAT:
		source = uatDeliverables[:]
	case PhaseK6:
		source = k6Deliverables[:]
	case PhasePentest:
		source = pentestDeliverables[:]
	case PhaseGoLive:
		source = goLiveDeliverables[:]
	case PhaseMaintenance:
		source = maintenanceDeliverables[:]
	default:
		return nil
	}

	return append([]string(nil), source...)
}
