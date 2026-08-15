package chunking

import (
	"strings"
	"testing"
)

type fakeCounter struct{}

func (fakeCounter) CountTokens(text string) int {
	return len(strings.Fields(text))
}

func newTestChunker(maxTokens, overlap int) *Chunker {
	return New([]string{"## "}, maxTokens, overlap, fakeCounter{})
}

func TestEmptyDoc(t *testing.T) {
	c := newTestChunker(100, 10)
	chunks, err := c.Chunk("doc1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty doc, got %d", len(chunks))
	}
}

func TestWhitespaceOnlyDoc(t *testing.T) {
	c := newTestChunker(100, 10)
	chunks, err := c.Chunk("doc1", "   \n  \n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for whitespace-only doc, got %d", len(chunks))
	}
}

func TestSingleSectionNoHeader(t *testing.T) {
	c := newTestChunker(100, 10)
	content := "This is a simple paragraph with no headers."
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Heading != "" {
		t.Errorf("expected empty heading, got %q", chunks[0].Heading)
	}
	if chunks[0].Content != content {
		t.Errorf("content mismatch:\n  got:  %q\n  want: %q", chunks[0].Content, content)
	}
	if chunks[0].DocID != "doc1" {
		t.Errorf("expected DocID doc1, got %q", chunks[0].DocID)
	}
}

func TestSplitsOnHeaders(t *testing.T) {
	c := New([]string{"## ", "# "}, 1000, 10, fakeCounter{})
	content := "# Intro\nSome intro text.\n## Section A\nContent A here.\n## Section B\nContent B here."
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	expected := []struct {
		heading string
		content string
	}{
		{"# Intro", "Some intro text."},
		{"## Section A", "Content A here."},
		{"## Section B", "Content B here."},
	}
	for i, exp := range expected {
		if chunks[i].Heading != exp.heading {
			t.Errorf("chunk[%d] heading: got %q, want %q", i, chunks[i].Heading, exp.heading)
		}
		if chunks[i].Content != exp.content {
			t.Errorf("chunk[%d] content: got %q, want %q", i, chunks[i].Content, exp.content)
		}
	}
}

func TestContentBeforeFirstHeader(t *testing.T) {
	c := newTestChunker(1000, 10)
	content := "Preamble text here.\n## First Section\nSection content."
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Heading != "" {
		t.Errorf("expected empty heading for preamble, got %q", chunks[0].Heading)
	}
	if chunks[0].Content != "Preamble text here." {
		t.Errorf("preamble content mismatch: got %q", chunks[0].Content)
	}
}

func TestChunkIndexSequential(t *testing.T) {
	c := newTestChunker(1000, 10)
	content := "## A\naaa\n## B\nbbb\n## C\nccc"
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, ch := range chunks {
		if ch.ChunkIndex != i {
			t.Errorf("chunk[%d] index: got %d, want %d", i, ch.ChunkIndex, i)
		}
	}
}

func TestOversizedSectionSplitWithOverlap(t *testing.T) {
	c := newTestChunker(7, 3)
	content := "## Big Section\nword1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12 word13 word14 word15"
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 15 tokens, max=7, overlap=3 → 3 chunks:
	// chunk0: word1..word7, chunk1: word5..word11, chunk2: word9..word15

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	chunk0Words := strings.Fields(chunks[0].Content)
	chunk1Words := strings.Fields(chunks[1].Content)
	overlapFromChunk0 := chunk0Words[len(chunk0Words)-3:]
	chunk1Prefix := chunk1Words[:3]

	for i := 0; i < 3; i++ {
		if overlapFromChunk0[i] != chunk1Prefix[i] {
			t.Errorf("overlap mismatch at position %d: chunk0 tail %q != chunk1 head %q",
				i, overlapFromChunk0[i], chunk1Prefix[i])
		}
	}

	for i, ch := range chunks {
		if ch.Heading != "## Big Section" {
			t.Errorf("chunk[%d] heading: got %q, want %q", i, ch.Heading, "## Big Section")
		}
	}
}

func TestMultipleSplitOnMarkers(t *testing.T) {
	c := New([]string{"## ", "# "}, 1000, 10, fakeCounter{})
	content := "# H1\npreamble\n## H2\nbody"
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Heading != "# H1" {
		t.Errorf("chunk[0] heading: got %q, want %q", chunks[0].Heading, "# H1")
	}
	if chunks[1].Heading != "## H2" {
		t.Errorf("chunk[1] heading: got %q, want %q", chunks[1].Heading, "## H2")
	}
}

func TestOversizedNoSplitOnMarker(t *testing.T) {
	c := newTestChunker(5, 2)
	content := "one two three four five six seven eight nine ten"
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for oversized plain text, got %d", len(chunks))
	}
	ch0Words := strings.Fields(chunks[0].Content)
	ch1Words := strings.Fields(chunks[1].Content)
	tail := ch0Words[len(ch0Words)-2:]
	head := ch1Words[:2]
	if tail[0] != head[0] || tail[1] != head[1] {
		t.Errorf("overlap mismatch: chunk0 tail %v, chunk1 head %v", tail, head)
	}
}

func TestNoNewContentAfterOverlap(t *testing.T) {
	c := newTestChunker(10, 3)
	content := "## Exact\none two three four five"
	chunks, err := c.Chunk("doc1", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (fits exactly), got %d", len(chunks))
	}
	if chunks[0].Content != "one two three four five" {
		t.Errorf("content mismatch: got %q", chunks[0].Content)
	}
}
