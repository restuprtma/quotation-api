package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func evidenceFor(t *testing.T, doc *MeetingNoteDocument, chunkID, text string) MeetingEvidence {
	t.Helper()
	for _, chunk := range doc.Chunks() {
		if chunk.ID != chunkID {
			continue
		}
		start := indexRunes(chunk.Text, text)
		require.GreaterOrEqual(t, start, 0)
		lineID := chunk.Lines[0].ID
		for _, line := range chunk.Lines {
			lineStart := line.StartRune - chunk.Lines[0].StartRune
			lineEnd := line.EndRune - chunk.Lines[0].StartRune
			if start >= lineStart && start+len([]rune(text)) <= lineEnd {
				lineID = line.ID
				break
			}
		}
		return MeetingEvidence{Refs: []string{chunk.ID, lineID}, StartOffset: start, EndOffset: start + len([]rune(text)), NormalizedSHA256: normalizedEvidenceHash(text), Support: EvidenceSupportExplicit}
	}
	t.Fatalf("chunk %s not found", chunkID)
	return MeetingEvidence{}
}

func indexRunes(s, needle string) int {
	index := strings.Index(s, needle)
	if index < 0 {
		return -1
	}
	return len([]rune(s[:index]))
}

func TestMeetingEvidenceCheckerTextNormalizesDiacriticsAndProducesSafeToken(t *testing.T) {
	doc, err := NewMeetingNoteDocument("Klien: Cafe\u0301\n", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	checker := NewMeetingEvidenceChecker(doc)
	evidence := evidenceFor(t, doc, "C001", "Café")

	got, err := checker.ValidateText(evidence, "CAFE")
	require.NoError(t, err)
	assert.Equal(t, "Café", got.Text)
	assert.True(t, domain.IsEvidenceReferenceToken(got.Token))
	assert.NotContains(t, got.Token, "Café")
}

func TestMeetingEvidenceCheckerUsesRuneOffsetsAndRejectsMismatchedHash(t *testing.T) {
	doc, err := NewMeetingNoteDocument("😀 nilai 15 juta\n", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	checker := NewMeetingEvidenceChecker(doc)
	evidence := evidenceFor(t, doc, "C001", "15 juta")

	money, _, err := checker.ValidateIndonesianMoney(evidence)
	require.NoError(t, err)
	assert.Equal(t, int64(15_000_000), money)
	evidence.NormalizedSHA256 = "wrong"
	_, _, err = checker.ValidateIndonesianMoney(evidence)
	assert.ErrorIs(t, err, ErrEvidenceHashMismatch)
}

func TestMeetingEvidenceCheckerParsesIndonesianMoneyAndRejectsInvalidValues(t *testing.T) {
	doc, err := NewMeetingNoteDocument("15 jt\n15 juta\nRp15.000.000\n15,5 juta\n0 juta\n-15 juta\n", MeetingNoteDocumentConfig{ChunkMaxRunes: 20})
	require.NoError(t, err)
	checker := NewMeetingEvidenceChecker(doc)
	want := map[string]int64{"15 jt": 15_000_000, "15 juta": 15_000_000, "Rp15.000.000": 15_000_000, "15,5 juta": 15_500_000}
	for raw, expected := range want {
		value, _, err := checker.ValidateIndonesianMoney(evidenceFor(t, doc, chunkForText(t, doc, raw), raw))
		require.NoError(t, err, raw)
		assert.Equal(t, expected, value, raw)
	}
	for _, raw := range []string{"0 juta", "-15 juta"} {
		_, _, err := checker.ValidateIndonesianMoney(evidenceFor(t, doc, chunkForText(t, doc, raw), raw))
		assert.ErrorIs(t, err, ErrEvidenceValueInvalid, raw)
	}
}

func TestMeetingEvidenceCheckerValidatesNumericDateDurationHeadcountAndPayment(t *testing.T) {
	doc, err := NewMeetingNoteDocument("durasi 3 bulan\ntim 2 orang\ntanggal 29 Februari 2024\nDP 30%\npelunasan 70%\n", MeetingNoteDocumentConfig{ChunkMaxRunes: 30})
	require.NoError(t, err)
	checker := NewMeetingEvidenceChecker(doc)

	durationEvidence := evidenceFor(t, doc, chunkForText(t, doc, "3 bulan"), "3 bulan")
	duration, _, err := checker.ValidateDurationMonths(durationEvidence)
	require.NoError(t, err)
	assert.Equal(t, 3, duration)
	headcount, _, err := checker.ValidateHeadcount(evidenceFor(t, doc, chunkForText(t, doc, "2 orang"), "2 orang"))
	require.NoError(t, err)
	assert.Equal(t, 2, headcount)
	date, _, err := checker.ValidateDate(evidenceFor(t, doc, chunkForText(t, doc, "29 Februari 2024"), "29 Februari 2024"))
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC), date)
	payments, _, err := checker.ValidatePaymentPercentages([]MeetingEvidence{evidenceFor(t, doc, chunkForText(t, doc, "30%"), "30%"), evidenceFor(t, doc, chunkForText(t, doc, "70%"), "70%")})
	require.NoError(t, err)
	assert.Equal(t, []int{30, 70}, payments)

	badDoc, err := NewMeetingNoteDocument("31 Februari 2024\n101%\n", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	bad := NewMeetingEvidenceChecker(badDoc)
	_, _, err = bad.ValidateDate(evidenceFor(t, badDoc, "C001", "31 Februari 2024"))
	assert.ErrorIs(t, err, ErrEvidenceValueInvalid)
	_, _, err = bad.ValidatePaymentPercentages([]MeetingEvidence{evidenceFor(t, badDoc, "C001", "101%")})
	assert.ErrorIs(t, err, ErrEvidenceValueInvalid)
}

func chunkForText(t *testing.T, doc *MeetingNoteDocument, text string) string {
	t.Helper()
	for _, chunk := range doc.Chunks() {
		if indexRunes(chunk.Text, text) >= 0 {
			return chunk.ID
		}
	}
	t.Fatalf("text %q missing", text)
	return ""
}
