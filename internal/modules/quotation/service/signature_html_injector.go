package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var idMonths = [...]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

var (
	reHeaderDate    = regexp.MustCompile(`(<div[^>]*data-field="issue_date_header"[^>]*>[\s\S]*?<span[^>]*data-role="metadata-value"[^>]*>)([^<]*)(</span>)`)
	reSigDate       = regexp.MustCompile(`(<span[^>]*data-field="issue_date"[^>]*>)([^<]*)(</span>)`)
	reMalangDate    = regexp.MustCompile(`(<section[^>]*data-block-type="signature"[^>]*>[\s\S]*?<p>Malang,\s*)([^<]+)(</p>)`)
	reDirectorCell  = regexp.MustCompile(`((?:<p>)?Hormat Kami,(?:<\/p>)?)([\s\S]*?)((?:<p>)?<strong>)`)
	reDirectorName  = regexp.MustCompile(`(<div[^>]*class="signature-stamp-box"[^>]*>[\s\S]*?<p><strong>)([^<]*)(</strong></p>)`)
	reDirectorTitle = regexp.MustCompile(`(<div[^>]*class="signature-stamp-box"[^>]*>[\s\S]*?</strong></p>\s*<p>)([^<]*)(</p>)`)
	reClientBox     = regexp.MustCompile(`(<div[^>]*class="client-signature-box"[^>]*>)([\s\S]*?)(<\/div>)`)
	reClientCell    = regexp.MustCompile(`((?:<p>)?Disetujui Oleh,(?:<\/p>)?)([\s\S]*?)((?:<p>)?<strong>)`)
)

func FormatIndonesianDate(t time.Time) string {
	day := t.Day()
	month := idMonths[t.Month()-1]
	year := t.Year()
	return fmt.Sprintf("%d %s %d", day, month, year)
}

func SyncDatesAndInjectDirectorSignature(htmlStr string, pubDate time.Time, dirName, dirTitle, dirSig, stamp string) string {
	dateStr := FormatIndonesianDate(pubDate)

	// 1. Sync header date: <div data-role="metadata-row" data-field="issue_date_header">...<span data-role="metadata-value">...</span>
	res := reHeaderDate.ReplaceAllString(htmlStr, `${1}`+dateStr+`${3}`)

	// 2. Sync signature block date: <span data-field="issue_date">...</span>
	if reSigDate.MatchString(res) {
		res = reSigDate.ReplaceAllString(res, `${1}`+dateStr+`${3}`)
	} else {
		// Fallback for templates with <p>Malang, <date></p> inside signature block
		res = reMalangDate.ReplaceAllString(res, `${1}`+dateStr+`${3}`)
	}

	// 3. Inject Director Signature & Stamp Box into "Hormat Kami" cell
	var innerImgs strings.Builder
	if dirSig != "" {
		innerImgs.WriteString(fmt.Sprintf(`<img src="%s" alt="TTD Direksi" class="director-signature-img" style="position: absolute; left: 0; bottom: 0; max-height: 85px; max-width: 180px; object-fit: contain; z-index: 1;" />`, dirSig))
	}
	if stamp != "" {
		innerImgs.WriteString(fmt.Sprintf(`<img src="%s" alt="Stempel Perusahaan" class="company-stamp-img" style="position: absolute; left: 55px; top: -5px; width: 110px; height: 110px; object-fit: contain; opacity: 0.88; transform: rotate(-8deg); mix-blend-mode: multiply; z-index: 2;" />`, stamp))
	}

	directorBox := fmt.Sprintf(`<div class="signature-stamp-box" style="position: relative; width: 220px; height: 110px; margin: 8px 0;">%s</div>`, innerImgs.String())

	// Replace Hormat Kami cell placeholder
	res = reDirectorCell.ReplaceAllString(res, `${1}`+directorBox+`${3}`)

	if dirName != "" {
		res = reDirectorName.ReplaceAllString(res, `${1}`+dirName+`${3}`)
	}
	if dirTitle != "" {
		res = reDirectorTitle.ReplaceAllString(res, `${1}`+dirTitle+`${3}`)
	}

	return res
}

func InjectClientSignature(htmlStr string, clientSig string) string {
	if clientSig == "" {
		return htmlStr
	}

	clientImg := fmt.Sprintf(`<img src="%s" alt="Tanda Tangan Klien" class="client-signature-img" style="max-height: 85px; max-width: 200px; object-fit: contain; margin: 8px 0;" />`, clientSig)

	// Inject into client-signature-box or replace Disetujui Oleh <br><br>
	if strings.Contains(htmlStr, `class="client-signature-box"`) {
		return reClientBox.ReplaceAllString(htmlStr, `${1}`+clientImg+`${3}`)
	}

	return reClientCell.ReplaceAllString(htmlStr, `${1}`+clientImg+`${3}`)
}
