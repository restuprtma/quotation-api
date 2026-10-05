package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const (
	defaultMeetingNoteChunkMaxRunes = 1200
	defaultFullDocumentMaxRunes     = 4096
	meetingNoteChunkOverlapLines    = 2
)

var (
	ErrMeetingNotesInvalidUTF8 = errors.New("meeting notes are not valid UTF-8")
	ErrMeetingNotesTooLong     = errors.New("meeting notes exceed the allowed length")
	ErrMeetingNoteConfig       = errors.New("meeting note document configuration is invalid")
)

// MeetingNoteDocumentConfig controls only deterministic, in-memory document
// preparation. Zero values use the domain note limit and conservative routing
// defaults. Chunk overlap is deliberately fixed at two lines by the pipeline
// contract, while the chunk size and full-document routing threshold are
// configurable.
type MeetingNoteDocumentConfig struct {
	MaxNotesRunes        int
	ChunkMaxRunes        int
	FullDocumentMaxRunes int
}

// MeetingNoteLine identifies a source line and its rune span in the canonical
// document. EndRune is exclusive.
type MeetingNoteLine struct {
	ID        string
	Text      string
	StartRune int
	EndRune   int
}

// MeetingNoteChunk is a line-aligned routing unit. Its offsets are always
// relative to Text, never to an evidence-normalized comparison string.
type MeetingNoteChunk struct {
	ID    string
	Text  string
	Lines []MeetingNoteLine

	StartRune int
	EndRune   int
}

func (c MeetingNoteChunk) LineIDs() []string {
	ids := make([]string, len(c.Lines))
	for i := range c.Lines {
		ids[i] = c.Lines[i].ID
	}
	return ids
}

// SliceRunes returns the canonical chunk span for valid exclusive offsets.
// Invalid offsets return an empty string so untrusted evidence cannot panic a
// worker before validation reports a safe defect code.
func (c MeetingNoteChunk) SliceRunes(startOffset, endOffset int) string {
	runes := []rune(c.Text)
	if startOffset < 0 || endOffset < startOffset || endOffset > len(runes) {
		return ""
	}
	return string(runes[startOffset:endOffset])
}

// MeetingNoteRoute contains only canonical chunks. Warning is an internal
// safe code; it intentionally carries no source text or provider detail.
type MeetingNoteRoute struct {
	Chunks       []MeetingNoteChunk
	FullDocument bool
	Fallback     bool
	Warning      string
	documentText string
}

func (r MeetingNoteRoute) ChunkIDs() []string {
	ids := make([]string, len(r.Chunks))
	for i := range r.Chunks {
		ids[i] = r.Chunks[i].ID
	}
	return ids
}

func (r MeetingNoteRoute) Text() string {
	if r.FullDocument {
		return r.documentText
	}
	parts := make([]string, len(r.Chunks))
	for i := range r.Chunks {
		parts[i] = r.Chunks[i].Text
	}
	return strings.Join(parts, "\n")
}

// MeetingNoteDocument is immutable after construction. It must remain local
// to a worker attempt; this primitive neither persists nor logs its contents.
type MeetingNoteDocument struct {
	canonical string
	hash      string
	lines     []MeetingNoteLine
	chunks    []MeetingNoteChunk
	config    MeetingNoteDocumentConfig
}

func NewMeetingNoteDocument(notes string, config MeetingNoteDocumentConfig) (*MeetingNoteDocument, error) {
	if !utf8.ValidString(notes) {
		return nil, ErrMeetingNotesInvalidUTF8
	}
	config, err := normalizeMeetingNoteDocumentConfig(config)
	if err != nil {
		return nil, err
	}

	canonical := canonicalMeetingNotes(notes)
	if len([]rune(canonical)) > config.MaxNotesRunes {
		return nil, ErrMeetingNotesTooLong
	}

	lines := buildMeetingNoteLines(canonical)
	chunks := buildMeetingNoteChunks(canonical, lines, config.ChunkMaxRunes)
	digest := sha256.Sum256([]byte(canonical))
	return &MeetingNoteDocument{
		canonical: canonical,
		hash:      hex.EncodeToString(digest[:]),
		lines:     lines,
		chunks:    chunks,
		config:    config,
	}, nil
}

func (d *MeetingNoteDocument) Canonical() string { return d.canonical }
func (d *MeetingNoteDocument) Hash() string      { return d.hash }

func (d *MeetingNoteDocument) Lines() []MeetingNoteLine {
	return append([]MeetingNoteLine(nil), d.lines...)
}

func (d *MeetingNoteDocument) LineIDs() []string {
	ids := make([]string, len(d.lines))
	for i := range d.lines {
		ids[i] = d.lines[i].ID
	}
	return ids
}

func (d *MeetingNoteDocument) Chunks() []MeetingNoteChunk {
	chunks := make([]MeetingNoteChunk, len(d.chunks))
	for i := range d.chunks {
		chunks[i] = cloneMeetingNoteChunk(d.chunks[i])
	}
	return chunks
}

// Reconstruct is deliberately line-based, so chunk overlap never duplicates
// source text during integrity checks.
func (d *MeetingNoteDocument) Reconstruct() string {
	parts := make([]string, len(d.lines))
	for i := range d.lines {
		parts[i] = d.lines[i].Text
	}
	return strings.Join(parts, "\n")
}

// Route returns the whole document for short notes. For long notes it resolves
// chunk IDs (C001) and line IDs/ranges (L0001 or L0001-L0004), adds exactly
// one neighboring chunk on both sides, and returns each chunk at most once.
// Empty or unknown long-note references use a bounded leading window instead
// of silently sending no context.
func (d *MeetingNoteDocument) Route(refs []string) MeetingNoteRoute {
	if len([]rune(d.canonical)) <= d.config.FullDocumentMaxRunes {
		return MeetingNoteRoute{Chunks: d.Chunks(), FullDocument: true, documentText: d.canonical}
	}

	selected := d.chunkIndexesForRefs(refs)
	if len(selected) == 0 {
		return MeetingNoteRoute{
			Chunks:   d.boundedLeadingWindow(),
			Fallback: true,
			Warning:  "ROUTING_FALLBACK",
		}
	}
	withNeighbors := make(map[int]struct{}, len(selected)*3)
	for index := range selected {
		withNeighbors[index] = struct{}{}
		if index > 0 {
			withNeighbors[index-1] = struct{}{}
		}
		if index+1 < len(d.chunks) {
			withNeighbors[index+1] = struct{}{}
		}
	}
	return MeetingNoteRoute{Chunks: d.chunksAt(withNeighbors)}
}

func normalizeMeetingNoteDocumentConfig(config MeetingNoteDocumentConfig) (MeetingNoteDocumentConfig, error) {
	if config.MaxNotesRunes == 0 {
		config.MaxNotesRunes = domain.MaxPromptMeetingNotes
	}
	if config.ChunkMaxRunes == 0 {
		config.ChunkMaxRunes = defaultMeetingNoteChunkMaxRunes
	}
	if config.FullDocumentMaxRunes == 0 {
		config.FullDocumentMaxRunes = defaultFullDocumentMaxRunes
	}
	if config.MaxNotesRunes < 0 || config.MaxNotesRunes > domain.MaxPromptMeetingNotes || config.ChunkMaxRunes <= 0 || config.FullDocumentMaxRunes <= 0 {
		return MeetingNoteDocumentConfig{}, fmt.Errorf("%w", ErrMeetingNoteConfig)
	}
	return config, nil
}

func canonicalMeetingNotes(notes string) string {
	notes = strings.ReplaceAll(notes, "\r\n", "\n")
	notes = strings.ReplaceAll(notes, "\r", "\n")
	notes = norm.NFC.String(notes)
	lines := strings.Split(notes, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

func buildMeetingNoteLines(canonical string) []MeetingNoteLine {
	parts := strings.Split(canonical, "\n")
	lines := make([]MeetingNoteLine, len(parts))
	position := 0
	for i, text := range parts {
		length := len([]rune(text))
		lines[i] = MeetingNoteLine{
			ID:        fmt.Sprintf("L%04d", i+1),
			Text:      text,
			StartRune: position,
			EndRune:   position + length,
		}
		position += length + 1 // canonical newline; harmless after the final line.
	}
	return lines
}

func buildMeetingNoteChunks(canonical string, lines []MeetingNoteLine, maxRunes int) []MeetingNoteChunk {
	if len(lines) == 0 {
		return nil
	}
	runes := []rune(canonical)
	chunks := make([]MeetingNoteChunk, 0, len(lines))
	for start := 0; start < len(lines); {
		end := start
		for end < len(lines) {
			candidateRunes := lines[end].EndRune - lines[start].StartRune
			if end > start && candidateRunes > maxRunes {
				break
			}
			end++
		}
		last := lines[end-1]
		first := lines[start]
		chunkLines := append([]MeetingNoteLine(nil), lines[start:end]...)
		chunks = append(chunks, MeetingNoteChunk{
			ID:        fmt.Sprintf("C%03d", len(chunks)+1),
			Text:      string(runes[first.StartRune:last.EndRune]),
			Lines:     chunkLines,
			StartRune: 0,
			EndRune:   last.EndRune - first.StartRune,
		})
		if end == len(lines) {
			break
		}
		next := end - meetingNoteChunkOverlapLines
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return chunks
}

func (d *MeetingNoteDocument) chunkIndexesForRefs(refs []string) map[int]struct{} {
	selected := make(map[int]struct{})
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		for index, chunk := range d.chunks {
			if ref == chunk.ID || chunkHasLineReference(chunk, ref) {
				selected[index] = struct{}{}
			}
		}
	}
	return selected
}

func chunkHasLineReference(chunk MeetingNoteChunk, ref string) bool {
	start, end, ok := parseMeetingLineReference(ref)
	if !ok {
		return false
	}
	for _, line := range chunk.Lines {
		lineNumber := meetingLineNumber(line.ID)
		if lineNumber >= start && lineNumber <= end {
			return true
		}
	}
	return false
}

func parseMeetingLineReference(ref string) (int, int, bool) {
	parts := strings.Split(ref, "-")
	if len(parts) > 2 {
		return 0, 0, false
	}
	start := meetingLineNumber(parts[0])
	end := start
	if len(parts) == 2 {
		end = meetingLineNumber(parts[1])
	}
	return start, end, start > 0 && end >= start
}

func meetingLineNumber(id string) int {
	var number int
	if _, err := fmt.Sscanf(id, "L%d", &number); err != nil || number <= 0 {
		return 0
	}
	return number
}

func (d *MeetingNoteDocument) chunksAt(indexes map[int]struct{}) []MeetingNoteChunk {
	ordered := make([]int, 0, len(indexes))
	for index := range indexes {
		ordered = append(ordered, index)
	}
	sort.Ints(ordered)
	chunks := make([]MeetingNoteChunk, 0, len(ordered))
	for _, index := range ordered {
		chunks = append(chunks, cloneMeetingNoteChunk(d.chunks[index]))
	}
	return chunks
}

func (d *MeetingNoteDocument) boundedLeadingWindow() []MeetingNoteChunk {
	selected := make(map[int]struct{})
	usedRunes := 0
	for index, chunk := range d.chunks {
		chunkRunes := len([]rune(chunk.Text))
		if index > 0 && usedRunes+chunkRunes > d.config.FullDocumentMaxRunes {
			break
		}
		selected[index] = struct{}{}
		usedRunes += chunkRunes
	}
	return d.chunksAt(selected)
}

func cloneMeetingNoteChunk(chunk MeetingNoteChunk) MeetingNoteChunk {
	chunk.Lines = append([]MeetingNoteLine(nil), chunk.Lines...)
	return chunk
}
