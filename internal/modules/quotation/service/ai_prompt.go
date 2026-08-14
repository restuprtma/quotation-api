package service

import (
	"fmt"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func buildSystemPrompt() string {
	return `Anda adalah asisten AI teknis. Tugas Anda adalah menyusun dokumen spesifikasi dan perencanaan perangkat lunak berdasarkan kebutuhan pengguna.
Anda HARUS menghasilkan output HANYA dalam format JSON valid (tanpa markdown blok tambahan seperti '''json).

Format JSON yang diwajibkan:
{
  "schema_version": "1.0",
  "narrative": {
    "introduction": "Deskripsi singkat tentang proyek...",
    "scope": ["Ruang lingkup 1", "Ruang lingkup 2"]
  },
  "product_or_project_detail": {
    "mode": "product", // isi dengan "product" atau "project"
    "modules": [       // Gunakan ini JIKA mode "product"
      {
        "name": "Nama Modul",
        "features": ["Fitur 1", "Fitur 2"]
      }
    ],
    "goals": ["Tujuan 1"],        // Gunakan ini JIKA mode "project"
    "deliverables": ["Hasil 1"],  // Gunakan ini JIKA mode "project"
    "milestones": [               // Gunakan ini JIKA mode "project"
      {"name": "Tahap 1", "duration": "2 weeks"}
    ],
    "team": [                     // Gunakan ini JIKA mode "project"
      {"role": "Developer", "count": 2}
    ]
  },
  "blueprint": {
    "title": "Blueprint",
    "summary": "Ringkasan sistem",
    "actors": [{"id": "user", "label": "User", "description": "Pengguna aplikasi"}],
    "diagrams": [
      {
        "type": "journey", // HANYA gunakan tipe yang diizinkan!
        "title": "Judul Diagram",
        "columns": [
           {"id": "col1", "phase_name": "Tahap 1", "order": 1, "description": "Deskripsi"}
        ]
      }
    ],
    "rules": ["Aturan 1"],
    "assumptions": ["Asumsi 1"],
    "edge_cases": ["Edge case 1"],
    "outputs": ["Output 1"]
  },
  "pricing_labels": ["Setup", "Langganan"]
}

Tipe diagram yang diizinkan HANYA: journey, architecture, state_machine, workflow, timeline, comparison.
Gunakan Bahasa Indonesia yang baku dan profesional.
Jangan menyisipkan HTML script/style, gunakan plain text sederhana.
Pastikan valid JSON yang bisa langsung di-parse (tanpa trailing comma, dsb).
`
}

func buildUserPrompt(req *dto.AIGenerateRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Client Name: %s\n", req.ClientName))
	sb.WriteString(fmt.Sprintf("Client Industry: %s\n", req.ClientIndustry))
	sb.WriteString(fmt.Sprintf("Project Type: %s\n", req.ProjectType))
	sb.WriteString(fmt.Sprintf("Subject: %s\n", req.Subject))
	sb.WriteString("Requirements:\n")
	for _, r := range req.ClientRequirements {
		sb.WriteString(fmt.Sprintf("- %s\n", r))
	}
	if req.UserNotes != "" {
		sb.WriteString(fmt.Sprintf("\nNotes:\n%s\n", req.UserNotes))
	}
	return sb.String()
}
