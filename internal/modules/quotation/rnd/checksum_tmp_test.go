package rnd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrintCommittedLegalChecksum(t *testing.T) {
	markdownBytes, err := os.ReadFile(filepath.Join("testdata", "quotation-skill", "legal-template-v1.md"))
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

