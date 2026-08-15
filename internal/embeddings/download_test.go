package embeddings

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestEnsureModel_DownloadsMissingFile(t *testing.T) {
	content := []byte("fake onnx model bytes")
	hash := sha256Hex(content)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write(content) //nolint:errcheck
	}))
	defer srv.Close()

	modelPath := filepath.Join(t.TempDir(), "model.onnx")

	err := EnsureModel(modelPath, srv.URL, hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}

	got, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("read model: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestEnsureModel_SHA256Mismatch(t *testing.T) {
	content := []byte("fake onnx model bytes")
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content) //nolint:errcheck
	}))
	defer srv.Close()

	modelPath := filepath.Join(t.TempDir(), "model.onnx")

	err := EnsureModel(modelPath, srv.URL, wrongHash)
	if err == nil {
		t.Fatal("expected error for SHA256 mismatch, got nil")
	}

	if _, statErr := os.Stat(modelPath); statErr == nil {
		t.Fatal("model file should not exist after hash mismatch")
	}
}

func TestEnsureModel_CorrectHashNoDownload(t *testing.T) {
	content := []byte("fake onnx model bytes")
	hash := sha256Hex(content)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write(content) //nolint:errcheck
	}))
	defer srv.Close()

	modelPath := filepath.Join(t.TempDir(), "model.onnx")

	if err := os.WriteFile(modelPath, content, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := EnsureModel(modelPath, srv.URL, hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hits.Load() != 0 {
		t.Fatalf("expected 0 hits (no download), got %d", hits.Load())
	}
}

func TestEnsureModel_ExistingFileWrongHash(t *testing.T) {
	wrongContent := []byte("stale model")
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("new content")) //nolint:errcheck
	}))
	defer srv.Close()

	modelPath := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(modelPath, wrongContent, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := EnsureModel(modelPath, srv.URL, wrongHash)
	if err == nil {
		t.Fatal("expected error for existing file with wrong hash")
	}
}

func TestEnsureModel_OfflineLocalFileExists(t *testing.T) {
	content := []byte("local model")
	modelPath := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(modelPath, content, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := EnsureModel(modelPath, "http://should-not-be-called.example.com/model.onnx", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("read model: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestEnsureModel_TempFilesCleanedOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data")) //nolint:errcheck
	}))
	defer srv.Close()

	modelDir := t.TempDir()
	modelPath := filepath.Join(modelDir, "model.onnx")

	err := EnsureModel(modelPath, srv.URL, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error")
	}

	entries, err := os.ReadDir(modelDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		t.Errorf("unexpected file left after failure: %s", e.Name())
	}
}

func TestEnsureModelDir_AllFilesDownloaded(t *testing.T) {
	files := map[string][]byte{
		"/onnx/model.onnx":      []byte("model graph"),
		"/onnx/model.onnx_data": []byte("model weights"),
		"/tokenizer.json":       []byte(`{"type":"tokenizer"}`),
		"/config.json":          []byte(`{"dim":768}`),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data) //nolint:errcheck
	}))
	defer srv.Close()

	modelDir := t.TempDir()
	modelFiles := []ModelFile{
		{LocalName: "model.onnx", RemotePath: "onnx/model.onnx"},
		{LocalName: "model.onnx_data", RemotePath: "onnx/model.onnx_data"},
		{LocalName: "tokenizer.json", RemotePath: "tokenizer.json"},
		{LocalName: "config.json", RemotePath: "config.json"},
	}

	err := EnsureModelDir(modelDir, srv.URL, modelFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, mf := range modelFiles {
		got, err := os.ReadFile(filepath.Join(modelDir, mf.LocalName))
		if err != nil {
			t.Fatalf("read %s: %v", mf.LocalName, err)
		}
		want := files["/"+mf.RemotePath]
		if string(got) != string(want) {
			t.Errorf("%s: got %q, want %q", mf.LocalName, got, want)
		}
	}
}

func TestEnsureModelDir_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tokenizer.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer srv.Close()

	modelDir := t.TempDir()
	modelFiles := []ModelFile{
		{LocalName: "model.onnx", RemotePath: "onnx/model.onnx"},
		{LocalName: "tokenizer.json", RemotePath: "tokenizer.json"},
		{LocalName: "config.json", RemotePath: "config.json"},
	}

	err := EnsureModelDir(modelDir, srv.URL, modelFiles)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}

	if _, err := os.Stat(filepath.Join(modelDir, "model.onnx")); err != nil {
		t.Error("model.onnx should exist despite tokenizer failure")
	}
	if _, err := os.Stat(filepath.Join(modelDir, "tokenizer.json")); err == nil {
		t.Error("tokenizer.json should not exist after 404")
	}
}

func TestEnsureModelDir_CreatesMissingDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data")) //nolint:errcheck
	}))
	defer srv.Close()

	modelDir := filepath.Join(t.TempDir(), "nested", "models", "embedding")
	modelFiles := []ModelFile{
		{LocalName: "model.onnx", RemotePath: "onnx/model.onnx"},
		{LocalName: "tokenizer.json", RemotePath: "tokenizer.json"},
	}

	err := EnsureModelDir(modelDir, srv.URL, modelFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(modelDir)
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory, got file")
	}

	for _, mf := range modelFiles {
		if _, err := os.Stat(filepath.Join(modelDir, mf.LocalName)); err != nil {
			t.Errorf("%s not created: %v", mf.LocalName, err)
		}
	}
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
