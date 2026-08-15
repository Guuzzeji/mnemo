package embeddings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/gomlx/go-huggingface/tokenizers/hftokenizer"
	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

type Embedder interface {
	Embed(text string) ([]float32, error)
	CountTokens(text string) int
}

type session interface {
	RunEmbedding(ctx context.Context, text string) ([]float32, error)
}

type embedder struct {
	session    session
	tokenizer  *hftokenizer.Tokenizer
	dimensions int
}

func NewEmbedder(ctx context.Context, modelDir string, dimensions int) (Embedder, error) {
	if _, err := os.Stat(modelDir); err != nil {
		return nil, fmt.Errorf("model dir: %w", err)
	}

	required := []string{"tokenizer.json", "config.json", "model.onnx", "model.onnx_data"}
	var missing []string
	for _, f := range required {
		if _, err := os.Stat(filepath.Join(modelDir, f)); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required files in %s: %s", modelDir, strings.Join(missing, ", "))
	}

	realSess, err := newRealSession(ctx, modelDir)
	if err != nil {
		return nil, err
	}

	tk, _ := loadTokenizer(modelDir)

	return &embedder{
		session:    realSess,
		tokenizer:  tk,
		dimensions: dimensions,
	}, nil
}

func (e *embedder) Embed(text string) ([]float32, error) {
	return e.session.RunEmbedding(context.Background(), text)
}

func (e *embedder) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	if e.tokenizer != nil {
		out := e.tokenizer.EncodeWithAnnotations(text)
		return len(out.IDs)
	}
	return len(text) / 4
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

type realSession struct {
	hugotSession *hugot.Session
	pipeline     *pipelines.FeatureExtractionPipeline
}

func (r *realSession) RunEmbedding(ctx context.Context, text string) ([]float32, error) {
	result, err := r.pipeline.RunPipeline(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return result.Embeddings[0], nil
}

func newRealSession(ctx context.Context, modelDir string) (session, error) {
	sess, err := hugot.NewGoSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("hugot session: %w", err)
	}

	config := hugot.FeatureExtractionConfig{
		ModelPath:    modelDir,
		Name:         "embeddings",
		OnnxFilename: "model.onnx",
		Options: []hugot.FeatureExtractionOption{
			pipelines.WithOutputName("sentence_embedding"),
		},
	}

	pipe, err := hugot.NewPipeline(sess, config)
	if err != nil {
		sess.Destroy()
		return nil, fmt.Errorf("feature extraction pipeline: %w", err)
	}

	return &realSession{
		hugotSession: sess,
		pipeline:     pipe,
	}, nil
}

func loadTokenizer(modelDir string) (*hftokenizer.Tokenizer, error) {
	tkBytes, err := os.ReadFile(filepath.Join(modelDir, "tokenizer.json"))
	if err != nil {
		return nil, fmt.Errorf("read tokenizer.json: %w", err)
	}
	tk, err := hftokenizer.NewFromContent(nil, tkBytes)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	err = tk.With(api.EncodeOptions{
		AddSpecialTokens: true,
	})
	if err != nil {
		return nil, fmt.Errorf("configure tokenizer: %w", err)
	}
	return tk, nil
}
