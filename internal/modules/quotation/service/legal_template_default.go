package service

const DefaultLegalTemplateVersion = 3

const DedicatedTaxClause = "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia."
const DedicatedInvoiceClause = "Tagihan dikirimkan sesuai tanggal invoice dan wajib dilunasi dalam jangka waktu H+14 (empat belas) hari kalender."
const DedicatedSuspensionClause = "Keterlambatan pembayaran berakibat pada penghentian sementara pekerjaan pengembangan dan pengunduran jadwal timeline tanpa penalti bagi Venturo."
const DedicatedBindingClause = "Dokumen penawaran disepakati sebagai bagian yang tidak terpisahkan dari Service Agreement Venturo."

var DedicatedExclusionClauses = [...]string{
	"Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.",
	"Percepatan pekerjaan di luar jam kerja reguler (malam hari, Sabtu, Minggu, atau hari libur nasional) dikenakan biaya lembur tersendiri.",
	"Lisensi pihak ketiga, credit API AI (OpenAI/Anthropic), Payment Gateway, WhatsApp API / SMS Gateway, domain, dan server cloud di luar paket yang disepakati.",
	"Penambahan fitur baru, perubahan alur proses yang telah di-approve, atau penundaan approval dari klien yang mengakibatkan masa kontrak molor.",
}

const DefaultLegalTemplateHTML = `<h2>Ketentuan</h2>` +
	`<h3>Pajak dan Invoice</h3>` +
	`<p>Nilai investasi yang tercantum dalam penawaran ini belum termasuk Pajak Pertambahan Nilai (PPN). Pengenaan dan besaran pajak mengikuti ketentuan perpajakan yang berlaku dan akan ditagihkan sesuai dokumen perpajakan yang sah.</p>` +
	`<p>Setiap invoice memiliki jatuh tempo pembayaran empat belas hari kalender (H+14) sejak tanggal invoice diterbitkan, kecuali terdapat kesepakatan tertulis lain yang disetujui oleh Para Pihak.</p>` +
	`<h3>Keterlambatan Pembayaran</h3>` +
	`<p>Apabila pembayaran melewati tanggal jatuh tempo, Penyedia berhak menghentikan sementara aktivitas pengembangan dan layanan terkait sampai kewajiban pembayaran dipenuhi. Penghentian sementara tersebut dapat menyebabkan perubahan atau penundaan timeline pekerjaan.</p>` +
	`<h3>Batasan dan Pengecualian</h3>` +
	`<p>Biaya transportasi, akomodasi, konsumsi, dan pengeluaran out-of-pocket lain yang timbul karena pekerjaan di luar lokasi kerja yang disepakati belum termasuk dalam nilai investasi, kecuali dinyatakan lain secara tertulis.</p>` +
	`<p>Pekerjaan onsite di luar lokasi kerja yang disepakati dan pekerjaan di luar jam kerja normal, termasuk lembur, hanya dilakukan berdasarkan persetujuan tertulis Para Pihak dan dapat dikenakan biaya tambahan.</p>` +
	`<h3>Biaya Pihak Ketiga</h3>` +
	`<p>Biaya layanan atau komponen pihak ketiga, termasuk tetapi tidak terbatas pada API berbayar, payment gateway, layanan AI, hosting, cloud service, domain, lisensi, dan kuota pemakaian, belum termasuk dalam nilai investasi kecuali dinyatakan lain secara tertulis dalam dokumen ini.</p>` +
	`<h3>Masa Berlaku Penawaran</h3>` +
	`<p>Penawaran ini berlaku selama tiga puluh (30) hari kalender sejak tanggal dokumen diterbitkan. Setelah melewati masa berlaku, nilai investasi dan ketentuan dalam penawaran ini dapat berubah.</p>` +
	`<h3>Penutup</h3>` +
	`<p>Dengan menandatangani dokumen ini, Para Pihak menyatakan telah membaca, memahami, dan menyetujui ruang lingkup pekerjaan, timeline, nilai investasi, serta ketentuan yang tercantum dalam penawaran ini. Pelaksanaan lebih lanjut mengikuti perjanjian layanan (Service Agreement) yang disepakati oleh Para Pihak.</p>`
