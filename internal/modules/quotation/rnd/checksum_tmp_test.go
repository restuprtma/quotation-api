package rnd

import (
	"os"
	"testing"
)

func TestPrintCommittedLegalChecksum(t *testing.T) {
	markdownBytes, err := os.ReadFile(`D:\venturo\Quotation-generator\docs\roadmap\rnd\quotation-skill\legal-template-v1.md`)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, legalHTML, err := extractLegalArtifact(string(markdownBytes))
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := hashHTMLFragment(legalHTML)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(checksum)
}
