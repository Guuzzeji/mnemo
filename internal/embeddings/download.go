package embeddings

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ModelFile describes one file to download into the model directory.
type ModelFile struct {
	LocalName  string // filename inside modelDir
	RemotePath string // path relative to repo root
	SHA256     string // empty = skip hash check
}

// EnsureModelDir downloads all files into modelDir, skipping already-present
// files whose hash matches (or have no hash). Returns combined errors for
// any files that failed; partially downloaded files are left in place.
func EnsureModelDir(modelDir, repoBaseURL string, files []ModelFile) error {
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}
	var errs []error
	for _, f := range files {
		dst := filepath.Join(modelDir, f.LocalName)
		url := repoBaseURL + "/" + f.RemotePath
		if err := EnsureModel(dst, url, f.SHA256); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.LocalName, err))
		}
	}
	return errors.Join(errs...)
}

func EnsureModel(modelPath, modelURL, sha256Hex string) error {
	if info, err := os.Stat(modelPath); err == nil && !info.IsDir() {
		if sha256Hex == "" {
			return nil
		}
		return verifyHash(modelPath, sha256Hex)
	}

	return download(modelPath, modelURL, sha256Hex)
}

func download(modelPath, modelURL, sha256Hex string) error {
	resp, err := http.Get(modelURL)
	if err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download model: HTTP %d", resp.StatusCode)
	}

	f, err := os.CreateTemp("", "model-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := f.Name()

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	_, err = io.Copy(writer, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write model: %w", err)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if sha256Hex != "" && got != sha256Hex {
		os.Remove(tmpPath)
		return fmt.Errorf("SHA256 mismatch: got %s, want %s", got, sha256Hex)
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename model: %w", err)
	}
	return nil
}

func verifyHash(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("SHA256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}
