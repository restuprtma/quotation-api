# Quotation Legal Template v1 Approved

## Template Metadata

```yaml
template_code: quotation-legal-v1
version: 1
status: approved
source: quotation_analisys.txt pattern summary
tax_behavior: excluded
language: id-ID
mutability_for_ai: immutable_after_approval
checksum: 68b08c62ecd5c9d989ea1c5a5a45813d11ac38bc60173afcf3b6c6878f394e4c
reviewer: Restu
approval_date: 2026-08-19
approval_decision: approved
skeleton_decision: approved
rate_card_decision: fixture-only
legal_decision: approved
representative_outputs: approved
```

Dokumen ini adalah template legal sintetis yang disetujui BA untuk kebutuhan R&D berdasarkan pola yang dirangkum dalam `quotation_analisys.txt`. Approval ini tidak menjadikannya nasihat hukum atau rate card production. Rate card tetap hanya untuk fixture mechanics. Representative outputs (simple dan table-heavy) telah disetujui oleh Restu pada 2026-08-20.

## Approval Gate

Approval BA untuk immutable legal block tercatat sebagai berikut:

1. Reviewer: `Restu`.
2. Tanggal approval: `2026-08-19` (legal template & tax behavior), `2026-08-20` (representative outputs).
3. Versi artefak: `v1`.
4. Keputusan tax behavior: `excluded` atau harga belum termasuk PPN.
5. Keputusan: skeleton `approved`, legal `approved`, rate card `fixture-only`, representative outputs `approved` (Restu, 2026-08-20).
6. Checksum normalized DOM: `68b08c62ecd5c9d989ea1c5a5a45813d11ac38bc60173afcf3b6c6878f394e4c`.

Generation harness MUST accept this file as approved only when the metadata and normalized DOM checksum match. `data-template-code="quotation-legal-v1"` alone is not evidence of approval.

## Exact Approved Text

Blok di bawah adalah exact approved text. AI tidak boleh memparafrase, meringkas, menerjemahkan, menambah, atau mengurangi kalimatnya setelah status approved. Merge field signature berada di luar legal prose dan diisi deterministic renderer.

### Opening Statement

Melalui penawaran ini, kami menyampaikan kesiapan tim Dedicated Programmer Squad untuk mendukung pelaksanaan pekerjaan sesuai ruang lingkup, timeline, dan rincian investasi yang tercantum dalam dokumen ini.

### Tax and Invoice

Nilai investasi yang tercantum dalam penawaran ini belum termasuk Pajak Pertambahan Nilai (PPN). Pengenaan dan besaran pajak mengikuti ketentuan perpajakan yang berlaku dan akan ditagihkan sesuai dokumen perpajakan yang sah.

Setiap invoice memiliki jatuh tempo pembayaran empat belas hari kalender (H+14) sejak tanggal invoice diterbitkan, kecuali terdapat kesepakatan tertulis lain yang disetujui oleh Para Pihak.

### Late Payment

Apabila pembayaran melewati tanggal jatuh tempo, Penyedia berhak menghentikan sementara aktivitas pengembangan dan layanan terkait sampai kewajiban pembayaran dipenuhi. Penghentian sementara tersebut dapat menyebabkan perubahan atau penundaan timeline pekerjaan.

### Out-of-Pocket, Onsite, and Overtime

Biaya transportasi, akomodasi, konsumsi, dan pengeluaran out-of-pocket lain yang timbul karena pekerjaan di luar lokasi kerja yang disepakati belum termasuk dalam nilai investasi, kecuali dinyatakan lain secara tertulis.

Pekerjaan onsite di luar lokasi kerja yang disepakati dan pekerjaan di luar jam kerja normal, termasuk lembur, hanya dilakukan berdasarkan persetujuan tertulis Para Pihak dan dapat dikenakan biaya tambahan.

### Third-Party Cost

Biaya layanan atau komponen pihak ketiga, termasuk tetapi tidak terbatas pada API berbayar, payment gateway, layanan AI, hosting, cloud service, domain, lisensi, dan kuota pemakaian, belum termasuk dalam nilai investasi kecuali dinyatakan lain secara tertulis dalam dokumen ini.

### Closing Statement

Dengan menandatangani dokumen ini, Para Pihak menyatakan telah membaca, memahami, dan menyetujui ruang lingkup pekerjaan, timeline, nilai investasi, serta ketentuan yang tercantum dalam penawaran ini. Pelaksanaan lebih lanjut mengikuti perjanjian layanan yang disepakati oleh Para Pihak.

## Deterministic HTML Block

Setelah BA menyetujui teks di atas, renderer wajib menghasilkan section berikut tanpa parafrase. Whitespace yang tidak bermakna boleh dinormalisasi untuk checksum DOM, tetapi text node dan urutan element harus tetap sama.

```html
<section data-block-type="legal" data-template-code="quotation-legal-v1" data-modified="false">
  <h2>Ketentuan</h2>
  <h3>Pernyataan Pembuka</h3>
  <p>Melalui penawaran ini, kami menyampaikan kesiapan tim Dedicated Programmer Squad untuk mendukung pelaksanaan pekerjaan sesuai ruang lingkup, timeline, dan rincian investasi yang tercantum dalam dokumen ini.</p>
  <h3>Pajak dan Invoice</h3>
  <p>Nilai investasi yang tercantum dalam penawaran ini belum termasuk Pajak Pertambahan Nilai (PPN). Pengenaan dan besaran pajak mengikuti ketentuan perpajakan yang berlaku dan akan ditagihkan sesuai dokumen perpajakan yang sah.</p>
  <p>Setiap invoice memiliki jatuh tempo pembayaran empat belas hari kalender (H+14) sejak tanggal invoice diterbitkan, kecuali terdapat kesepakatan tertulis lain yang disetujui oleh Para Pihak.</p>
  <h3>Keterlambatan Pembayaran</h3>
  <p>Apabila pembayaran melewati tanggal jatuh tempo, Penyedia berhak menghentikan sementara aktivitas pengembangan dan layanan terkait sampai kewajiban pembayaran dipenuhi. Penghentian sementara tersebut dapat menyebabkan perubahan atau penundaan timeline pekerjaan.</p>
  <h3>Out-of-Pocket, Onsite, dan Lembur</h3>
  <p>Biaya transportasi, akomodasi, konsumsi, dan pengeluaran out-of-pocket lain yang timbul karena pekerjaan di luar lokasi kerja yang disepakati belum termasuk dalam nilai investasi, kecuali dinyatakan lain secara tertulis.</p>
  <p>Pekerjaan onsite di luar lokasi kerja yang disepakati dan pekerjaan di luar jam kerja normal, termasuk lembur, hanya dilakukan berdasarkan persetujuan tertulis Para Pihak dan dapat dikenakan biaya tambahan.</p>
  <h3>Biaya Pihak Ketiga</h3>
  <p>Biaya layanan atau komponen pihak ketiga, termasuk tetapi tidak terbatas pada API berbayar, payment gateway, layanan AI, hosting, cloud service, domain, lisensi, dan kuota pemakaian, belum termasuk dalam nilai investasi kecuali dinyatakan lain secara tertulis dalam dokumen ini.</p>
  <h3>Penutup</h3>
  <p>Dengan menandatangani dokumen ini, Para Pihak menyatakan telah membaca, memahami, dan menyetujui ruang lingkup pekerjaan, timeline, nilai investasi, serta ketentuan yang tercantum dalam penawaran ini. Pelaksanaan lebih lanjut mengikuti perjanjian layanan yang disepakati oleh Para Pihak.</p>
</section>
```

## Immutable and Restore Rules

- Approved legal section MUST be supplied to AI as immutable content.
- AI MUST copy the approved section exactly and MUST NOT generate a substitute.
- Backend MUST compute and store a normalized DOM checksum of the approved source.
- Any text, element, order, marker, or attribute mutation in generated legal section MUST fail security validation.
- BA may unlock the legal section only after explicit confirmation in the editor.
- An unlocked edit MUST set `data-modified="true"` and create an auditable commercial/legal change marker.
- Restore action MUST replace the complete legal subtree with the approved source, reset `data-modified="false"`, and recompute the checksum.
- Restore MUST NOT use an AI rewrite or a partial search-and-replace operation.

## Signature Merge Fields

Signature names are deterministic fields and are not AI-authored legal prose:

```text
{{vendor_representative_name}}
{{vendor_company_name}}
{{client_representative_name}}
{{client_company_name}}
{{signature_city}}
{{signature_date}}
```

These merge fields MUST be resolved before final client-facing output. They MUST NOT remain in saved HTML, and they MUST NOT be filled with real client data in R&D fixtures.

## Evidence Notes

The candidate follows recurring evidence patterns for opening statement, tax/PPN statement, H+14 invoice due date, temporary suspension after late payment, out-of-pocket/onsite/overtime, third-party costs, closing statement, and signatures. The evidence did not itself establish an approved legal wording. BA approval is therefore a hard prerequisite, not implied by this candidate.
