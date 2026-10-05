package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestMeetingNoteDocumentCanonicalHashNormalizesOnlyFormattingEquivalence(t *testing.T) {
	left, err := NewMeetingNoteDocument("Cafe\u0301\r\nScope  \rPIC", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	right, err := NewMeetingNoteDocument("Café\nScope\nPIC", MeetingNoteDocumentConfig{})
	require.NoError(t, err)

	assert.Equal(t, "Café\nScope\nPIC", left.Canonical())
	assert.Equal(t, left.Hash(), right.Hash())

	changed, err := NewMeetingNoteDocument("Café\nScope changed\nPIC", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	assert.NotEqual(t, left.Hash(), changed.Hash())
}

func TestMeetingNoteDocumentUsesStableIDsAndRuneOffsets(t *testing.T) {
	doc, err := NewMeetingNoteDocument("alpha\nβeta\n😀x", MeetingNoteDocumentConfig{ChunkMaxRunes: 6})
	require.NoError(t, err)

	assert.Equal(t, []MeetingNoteLine{
		{ID: "L0001", Text: "alpha", StartRune: 0, EndRune: 5},
		{ID: "L0002", Text: "βeta", StartRune: 6, EndRune: 10},
		{ID: "L0003", Text: "😀x", StartRune: 11, EndRune: 13},
	}, doc.Lines())
	chunks := doc.Chunks()
	require.Len(t, chunks, 3)
	assert.Equal(t, "C001", chunks[0].ID)
	assert.Equal(t, "C002", chunks[1].ID)
	assert.Equal(t, "βeta", chunks[1].Text)
	assert.Equal(t, 0, chunks[1].StartRune)
	assert.Equal(t, 4, chunks[1].EndRune)
	assert.Equal(t, "eta", chunks[1].SliceRunes(1, 4))
	assert.Equal(t, "😀", chunks[2].SliceRunes(0, 1))
}

func TestMeetingNoteDocumentChunksAtLineBoundariesWithTwoLineOverlapAndNeighbors(t *testing.T) {
	doc, err := NewMeetingNoteDocument("aa\nbb\ncc\ndd\nee", MeetingNoteDocumentConfig{ChunkMaxRunes: 10, FullDocumentMaxRunes: 1})
	require.NoError(t, err)

	chunks := doc.Chunks()
	require.Len(t, chunks, 3)
	assert.Equal(t, []string{"L0001", "L0002", "L0003"}, chunks[0].LineIDs())
	assert.Equal(t, []string{"L0002", "L0003", "L0004"}, chunks[1].LineIDs())
	assert.Equal(t, []string{"L0003", "L0004", "L0005"}, chunks[2].LineIDs())

	route := doc.Route([]string{"C002", "L0003-L0004", "C002"})
	assert.False(t, route.FullDocument)
	assert.Equal(t, []string{"C001", "C002", "C003"}, route.ChunkIDs())
	assert.Equal(t, "aa\nbb\ncc\ndd\nee", doc.Reconstruct())
}

func TestMeetingNoteDocumentReturnsFullDocumentBelowRoutingThreshold(t *testing.T) {
	doc, err := NewMeetingNoteDocument("one\ntwo\nthree", MeetingNoteDocumentConfig{ChunkMaxRunes: 5, FullDocumentMaxRunes: 100})
	require.NoError(t, err)

	route := doc.Route([]string{"C999"})
	assert.True(t, route.FullDocument)
	assert.Equal(t, doc.Canonical(), route.Text())
	assert.Equal(t, []string{"C001", "C002", "C003"}, route.ChunkIDs())
}

func TestMeetingNoteDocumentFullRouteDoesNotDuplicateOverlapText(t *testing.T) {
	doc, err := NewMeetingNoteDocument("aa\nbb\ncc\ndd", MeetingNoteDocumentConfig{ChunkMaxRunes: 10, FullDocumentMaxRunes: 100})
	require.NoError(t, err)

	route := doc.Route([]string{"C002"})
	assert.True(t, route.FullDocument)
	assert.Equal(t, doc.Canonical(), route.Text())
}

func TestMeetingNoteDocumentPreservesBlankLinesAndAllowsLongSingleLineAsOneChunk(t *testing.T) {
	doc, err := NewMeetingNoteDocument("first\n\n"+strings.Repeat("😀", 8)+"\nlast  ", MeetingNoteDocumentConfig{ChunkMaxRunes: 3})
	require.NoError(t, err)

	assert.Equal(t, "first\n\n"+strings.Repeat("😀", 8)+"\nlast", doc.Canonical())
	assert.Equal(t, []string{"L0001", "L0002", "L0003", "L0004"}, doc.LineIDs())
	assert.Contains(t, doc.Chunks()[2].Text, strings.Repeat("😀", 8))
}

func TestMeetingNoteDocumentRejectsMalformedUTF8AndExcessiveNotes(t *testing.T) {
	_, err := NewMeetingNoteDocument(string([]byte{'o', 'k', 0xff}), MeetingNoteDocumentConfig{})
	assert.ErrorIs(t, err, ErrMeetingNotesInvalidUTF8)

	_, err = NewMeetingNoteDocument("12345", MeetingNoteDocumentConfig{MaxNotesRunes: 4})
	assert.ErrorIs(t, err, ErrMeetingNotesTooLong)

	_, err = NewMeetingNoteDocument(strings.Repeat("x", domain.MaxPromptMeetingNotes+1), MeetingNoteDocumentConfig{})
	assert.ErrorIs(t, err, ErrMeetingNotesTooLong)

	_, err = NewMeetingNoteDocument("ok", MeetingNoteDocumentConfig{MaxNotesRunes: domain.MaxPromptMeetingNotes + 1})
	assert.ErrorIs(t, err, ErrMeetingNoteConfig)
}

func FuzzMeetingNoteDocumentNeverPanicsAndReconstructsCanonicalDocument(f *testing.F) {
	f.Add("Cafe\u0301\r\n😀\n")
	f.Add("\n\nlong line")
	f.Fuzz(func(t *testing.T, notes string) {
		doc, err := NewMeetingNoteDocument(notes, MeetingNoteDocumentConfig{ChunkMaxRunes: 7, FullDocumentMaxRunes: 1})
		if err != nil {
			return
		}
		assert.Equal(t, doc.Canonical(), doc.Reconstruct())
		for _, chunk := range doc.Chunks() {
			assert.Equal(t, len([]rune(chunk.Text)), chunk.EndRune)
			assert.LessOrEqual(t, chunk.StartRune, chunk.EndRune)
		}
	})
}
