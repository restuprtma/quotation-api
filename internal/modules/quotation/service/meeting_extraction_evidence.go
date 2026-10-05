package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	ErrEvidenceInvalid      = errors.New("meeting evidence is invalid")
	ErrEvidenceReference    = fmt.Errorf("%w: reference", ErrEvidenceInvalid)
	ErrEvidenceOffset       = fmt.Errorf("%w: offset", ErrEvidenceInvalid)
	ErrEvidenceHashMismatch = fmt.Errorf("%w: normalized hash mismatch", ErrEvidenceInvalid)
	ErrEvidenceValueInvalid = fmt.Errorf("%w: value", ErrEvidenceInvalid)
)

const (
	EvidenceSupportExplicit = "explicit"
	EvidenceSupportInferred = "inferred"
)

// MeetingEvidence is the provider's non-quoted evidence envelope. Offsets are
// rune offsets relative to the named canonical chunk, never byte offsets.
type MeetingEvidence struct {
	Refs             []string `json:"refs"`
	StartOffset      int      `json:"start_offset"`
	EndOffset        int      `json:"end_offset"`
	NormalizedSHA256 string   `json:"normalized_sha256"`
	Support          string   `json:"support"`
}

// ValidatedEvidence contains no raw notes except for the in-memory Text used
// by callers to parse a typed fact. Persist Token, not Text.
type ValidatedEvidence struct {
	Token   string
	Text    string
	Support string
}

type MeetingEvidenceChecker struct{ document *MeetingNoteDocument }

func NewMeetingEvidenceChecker(document *MeetingNoteDocument) *MeetingEvidenceChecker {
	return &MeetingEvidenceChecker{document: document}
}

func (c *MeetingEvidenceChecker) ValidateText(evidence MeetingEvidence, expected string) (ValidatedEvidence, error) {
	validated, err := c.validate(evidence)
	if err != nil {
		return ValidatedEvidence{}, err
	}
	if normalizedEvidenceText(validated.Text) != normalizedEvidenceText(expected) {
		return ValidatedEvidence{}, fmt.Errorf("%w: text does not match", ErrEvidenceValueInvalid)
	}
	return validated, nil
}

func (c *MeetingEvidenceChecker) ValidatePositiveInteger(evidence MeetingEvidence) (int, ValidatedEvidence, error) {
	validated, err := c.validate(evidence)
	if err != nil {
		return 0, ValidatedEvidence{}, err
	}
	value, ok := parsePositiveInteger(validated.Text)
	if !ok {
		return 0, ValidatedEvidence{}, fmt.Errorf("%w: positive integer", ErrEvidenceValueInvalid)
	}
	return value, validated, nil
}

func (c *MeetingEvidenceChecker) ValidateDurationMonths(evidence MeetingEvidence) (int, ValidatedEvidence, error) {
	validated, err := c.validate(evidence)
	if err != nil {
		return 0, ValidatedEvidence{}, err
	}
	matches := durationMonthsPattern.FindStringSubmatch(normalizedEvidenceText(validated.Text))
	if len(matches) != 3 {
		return 0, ValidatedEvidence{}, fmt.Errorf("%w: duration months", ErrEvidenceValueInvalid)
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil || value <= 0 {
		return 0, ValidatedEvidence{}, fmt.Errorf("%w: duration months", ErrEvidenceValueInvalid)
	}
	return value, validated, nil
}

func (c *MeetingEvidenceChecker) ValidateHeadcount(evidence MeetingEvidence) (int, ValidatedEvidence, error) {
	validated, err := c.validate(evidence)
	if err != nil {
		return 0, ValidatedEvidence{}, err
	}
	matches := headcountPattern.FindStringSubmatch(normalizedEvidenceText(validated.Text))
	if len(matches) != 3 {
		return 0, ValidatedEvidence{}, fmt.Errorf("%w: headcount", ErrEvidenceValueInvalid)
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil || value <= 0 {
		return 0, ValidatedEvidence{}, fmt.Errorf("%w: headcount", ErrEvidenceValueInvalid)
	}
	return value, validated, nil
}

func (c *MeetingEvidenceChecker) ValidateDate(evidence MeetingEvidence) (time.Time, ValidatedEvidence, error) {
	validated, err := c.validate(evidence)
	if err != nil {
		return time.Time{}, ValidatedEvidence{}, err
	}
	matches := indonesianDatePattern.FindStringSubmatch(normalizedEvidenceText(validated.Text))
	if len(matches) != 4 {
		return time.Time{}, ValidatedEvidence{}, fmt.Errorf("%w: date", ErrEvidenceValueInvalid)
	}
	day, _ := strconv.Atoi(matches[1])
	year, _ := strconv.Atoi(matches[3])
	month, ok := evidenceIndonesianMonths[matches[2]]
	if !ok || day < 1 || year < 1 {
		return time.Time{}, ValidatedEvidence{}, fmt.Errorf("%w: date", ErrEvidenceValueInvalid)
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || date.Month() != month || date.Day() != day {
		return time.Time{}, ValidatedEvidence{}, fmt.Errorf("%w: date", ErrEvidenceValueInvalid)
	}
	return date, validated, nil
}

func (c *MeetingEvidenceChecker) ValidatePaymentPercentages(evidences []MeetingEvidence) ([]int, []ValidatedEvidence, error) {
	if len(evidences) == 0 {
		return nil, nil, fmt.Errorf("%w: payment percentages", ErrEvidenceValueInvalid)
	}
	values := make([]int, 0, len(evidences))
	validated := make([]ValidatedEvidence, 0, len(evidences))
	total := 0
	for _, evidence := range evidences {
		item, err := c.validate(evidence)
		if err != nil {
			return nil, nil, err
		}
		matches := paymentPercentPattern.FindStringSubmatch(strings.TrimSpace(item.Text))
		if len(matches) != 2 {
			return nil, nil, fmt.Errorf("%w: payment percentage", ErrEvidenceValueInvalid)
		}
		value, _ := strconv.Atoi(matches[1])
		if value < 0 || value > 100 {
			return nil, nil, fmt.Errorf("%w: payment percentage", ErrEvidenceValueInvalid)
		}
		values, validated, total = append(values, value), append(validated, item), total+value
	}
	if total != 100 {
		return nil, nil, fmt.Errorf("%w: payment percentage total", ErrEvidenceValueInvalid)
	}
	return values, validated, nil
}

func (c *MeetingEvidenceChecker) ValidateIndonesianMoney(evidence MeetingEvidence) (int64, ValidatedEvidence, error) {
	validated, err := c.validate(evidence)
	if err != nil {
		return 0, ValidatedEvidence{}, err
	}
	value, ok := parseIndonesianMoney(strings.ToLower(norm.NFC.String(validated.Text)))
	if !ok || value <= 0 {
		return 0, ValidatedEvidence{}, fmt.Errorf("%w: Indonesian money", ErrEvidenceValueInvalid)
	}
	return value, validated, nil
}

func (c *MeetingEvidenceChecker) validate(evidence MeetingEvidence) (ValidatedEvidence, error) {
	if c == nil || c.document == nil {
		return ValidatedEvidence{}, ErrEvidenceReference
	}
	if evidence.Support != EvidenceSupportExplicit && evidence.Support != EvidenceSupportInferred {
		return ValidatedEvidence{}, fmt.Errorf("%w: support", ErrEvidenceValueInvalid)
	}
	chunk, lineRange, ok := c.resolveChunk(evidence.Refs)
	if !ok {
		return ValidatedEvidence{}, ErrEvidenceReference
	}
	lineStart, lineEnd, _ := parseMeetingLineReference(lineRange)
	chunkStart := chunk.Lines[0].StartRune
	selectedStart := -1
	selectedEnd := -1
	for _, line := range chunk.Lines {
		number := meetingLineNumber(line.ID)
		if number == lineStart {
			selectedStart = line.StartRune - chunkStart
		}
		if number == lineEnd {
			selectedEnd = line.EndRune - chunkStart
		}
	}
	if selectedStart < 0 || selectedEnd < selectedStart || evidence.StartOffset < selectedStart || evidence.EndOffset > selectedEnd {
		return ValidatedEvidence{}, ErrEvidenceOffset
	}
	text := chunk.SliceRunes(evidence.StartOffset, evidence.EndOffset)
	if text == "" || evidence.EndOffset <= evidence.StartOffset {
		return ValidatedEvidence{}, ErrEvidenceOffset
	}
	actualHash := normalizedEvidenceHash(text)
	if !strings.EqualFold(strings.TrimSpace(evidence.NormalizedSHA256), actualHash) {
		return ValidatedEvidence{}, ErrEvidenceHashMismatch
	}
	return ValidatedEvidence{Token: domain.NewEvidenceReferenceToken(chunk.ID, lineRange, evidence.StartOffset, evidence.EndOffset, actualHash), Text: text, Support: evidence.Support}, nil
}

func (c *MeetingEvidenceChecker) resolveChunk(refs []string) (MeetingNoteChunk, string, bool) {
	var chunk *MeetingNoteChunk
	lineStart, lineEnd := 0, 0
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		for _, candidate := range c.document.Chunks() {
			if candidate.ID == ref {
				if chunk != nil && chunk.ID != candidate.ID {
					return MeetingNoteChunk{}, "", false
				}
				copy := candidate
				chunk = &copy
			}
		}
		if start, end, isLine := parseMeetingLineReference(ref); isLine {
			if lineStart == 0 || start < lineStart {
				lineStart = start
			}
			if end > lineEnd {
				lineEnd = end
			}
		}
	}
	if chunk == nil {
		return MeetingNoteChunk{}, "", false
	}
	if lineStart == 0 {
		lineStart, lineEnd = meetingLineNumber(chunk.Lines[0].ID), meetingLineNumber(chunk.Lines[len(chunk.Lines)-1].ID)
	}
	for n := lineStart; n <= lineEnd; n++ {
		found := false
		for _, line := range chunk.Lines {
			if meetingLineNumber(line.ID) == n {
				found = true
				break
			}
		}
		if !found {
			return MeetingNoteChunk{}, "", false
		}
	}
	if lineStart == lineEnd {
		return *chunk, fmt.Sprintf("L%04d", lineStart), true
	}
	return *chunk, fmt.Sprintf("L%04d-L%04d", lineStart, lineEnd), true
}

var (
	durationMonthsPattern    = regexp.MustCompile(`^([0-9]+) (bulan|bln)$`)
	headcountPattern         = regexp.MustCompile(`^([0-9]+) (orang|person|org)$`)
	paymentPercentPattern    = regexp.MustCompile(`^([0-9]{1,3})%$`)
	indonesianDatePattern    = regexp.MustCompile(`^([0-9]{1,2}) (januari|februari|maret|april|mei|juni|juli|agustus|september|oktober|november|desember) ([0-9]{4})$`)
	evidenceIndonesianMonths = map[string]time.Month{"januari": time.January, "februari": time.February, "maret": time.March, "april": time.April, "mei": time.May, "juni": time.June, "juli": time.July, "agustus": time.August, "september": time.September, "oktober": time.October, "november": time.November, "desember": time.December}
)

func parsePositiveInteger(text string) (int, bool) {
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(strings.TrimSpace(text)) {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(text))
	return value, err == nil && value > 0
}

func parseIndonesianMoney(text string) (int64, bool) {
	text = strings.ReplaceAll(strings.TrimSpace(text), " ", "")
	text = strings.TrimPrefix(text, "rp")
	text = strings.TrimPrefix(text, "rp.")
	multiplier := int64(1)
	if strings.HasSuffix(text, "juta") {
		text, multiplier = strings.TrimSuffix(text, "juta"), 1_000_000
	} else if strings.HasSuffix(text, "jt") {
		text, multiplier = strings.TrimSuffix(text, "jt"), 1_000_000
	}
	if strings.HasPrefix(text, "-") || text == "" {
		return 0, false
	}
	if multiplier == 1 {
		if !regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{3})+$|^[0-9]+$`).MatchString(text) {
			return 0, false
		}
		text = strings.ReplaceAll(text, ".", "")
		value, err := strconv.ParseInt(text, 10, 64)
		return value, err == nil
	}
	if !regexp.MustCompile(`^[0-9]+(,[0-9]+)?$`).MatchString(text) {
		return 0, false
	}
	parts := strings.Split(text, ",")
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	value := whole * multiplier
	if len(parts) == 2 {
		fraction, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, false
		}
		for n := len(parts[1]); n < 6; n++ {
			fraction *= 10
		}
		value += fraction
	}
	return value, true
}

func normalizedEvidenceHash(text string) string {
	digest := sha256.Sum256([]byte(normalizedEvidenceText(text)))
	return hex.EncodeToString(digest[:])
}

func normalizedEvidenceText(text string) string {
	text = norm.NFD.String(strings.ToLower(text))
	var b strings.Builder
	space := false
	for _, r := range text {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return b.String()
}
