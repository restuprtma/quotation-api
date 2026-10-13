package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reClientBox  = regexp.MustCompile(`(<div[^>]*class="client-signature-box"[^>]*>)([\s\S]*?)(<\/div>)`)
	reClientCell = regexp.MustCompile(`((?:<p>)?Disetujui Oleh,(?:<\/p>)?)([\s\S]*?)((?:<p>)?<strong>)`)
)

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
