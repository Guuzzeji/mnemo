package embeddings

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func isMissingFileError(err error) bool {
	return strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "required file")
}

type fakeSession struct {
	embedFunc func(ctx context.Context, text string) ([]float32, error)
}

func (f *fakeSession) RunEmbedding(ctx context.Context, text string) ([]float32, error) {
	return f.embedFunc(ctx, text)
}

func TestNewEmbedder_MissingModelDir(t *testing.T) {
	_, err := NewEmbedder(context.Background(), "/nonexistent/path", 768)
	if err == nil {
		t.Fatal("expected error for missing model dir")
	}
}

func TestNewEmbedder_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "tokenizer.json"), "{}"); err != nil {
		t.Fatal(err)
	}
	_, err := NewEmbedder(context.Background(), dir, 768)
	if err == nil {
		t.Fatal("expected error for missing model files")
	}
}

func TestNewEmbedder_AllFilesPresent(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"tokenizer.json", "config.json", "model.onnx", "model.onnx_data"} {
		if err := writeFile(filepath.Join(dir, f), "{}"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := NewEmbedder(context.Background(), dir, 768)
	if err != nil {
		if isMissingFileError(err) {
			t.Fatalf("file validation should have passed: %v", err)
		}
	}
}

func TestEmbed_CorrectDimension(t *testing.T) {
	wantDim := 384
	e := &embedder{
		session: &fakeSession{
			embedFunc: func(ctx context.Context, text string) ([]float32, error) {
				vec := make([]float32, wantDim)
				for i := range vec {
					vec[i] = float32(i)
				}
				return vec, nil
			},
		},
		dimensions: wantDim,
	}

	vec, err := e.Embed("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != wantDim {
		t.Fatalf("got dimension %d, want %d", len(vec), wantDim)
	}
}

func TestEmbed_ReturnsCorrectValues(t *testing.T) {
	e := &embedder{
		session: &fakeSession{
			embedFunc: func(ctx context.Context, text string) ([]float32, error) {
				return []float32{0.1, 0.2, 0.3}, nil
			},
		},
		dimensions: 3,
	}

	vec, err := e.Embed("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("got dimension %d, want 3", len(vec))
	}
	for i, want := range []float32{0.1, 0.2, 0.3} {
		if vec[i] != want {
			t.Errorf("vec[%d] = %f, want %f", i, vec[i], want)
		}
	}
}

func TestCountTokens_Heuristic(t *testing.T) {
	e := &embedder{dimensions: 768}
	tests := []struct {
		text string
		want int
	}{
		{"hello world", 2},
		{"", 0},
		{"a", 0},
		{"abcd", 1},
		{"hello world foo bar", 4},
	}
	for _, tt := range tests {
		got := e.CountTokens(tt.text)
		if got != tt.want {
			t.Errorf("CountTokens(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}
