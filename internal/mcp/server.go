package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Guuzzeji/ai-shared-memory/internal/config"
	"github.com/Guuzzeji/ai-shared-memory/internal/docs"
	"github.com/Guuzzeji/ai-shared-memory/internal/embeddings"
	"github.com/Guuzzeji/ai-shared-memory/internal/index"
	"github.com/Guuzzeji/ai-shared-memory/internal/logstore"
	"github.com/Guuzzeji/ai-shared-memory/internal/sync"
	"github.com/Guuzzeji/ai-shared-memory/internal/taxonomy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

type Deps struct {
	Config   *config.Config
	Log      *logstore.Store
	Docs     *docs.Store
	Index    *index.Index
	Syncer   *sync.Syncer
	Taxonomy *taxonomy.Taxonomy
	Embedder embeddings.Embedder
}

type server struct {
	deps Deps
}

func NewServer(deps Deps) (*mcp.Server, error) {
	impl := &mcp.Implementation{Name: "mnemo", Version: "0.1.0"}
	s := mcp.NewServer(impl, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{},
	})
	h := &server{deps: deps}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "semantic_search",
		Description: "Semantic search over the memory index. Query terms are expanded through taxonomy aliases; results are ranked by cosine similarity and filtered to active docs.",
	}, h.semanticSearch)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "append_memory",
		Description: "Append a memory entry to the JSONL log and apply markdown updates (create or append_section). Server sets log_id, timestamp, author. Unknown tags are rejected.",
	}, h.appendMemory)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_memory",
		Description: "Return the full Markdown document for a doc_id.",
	}, h.getMemory)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_memories",
		Description: "List doc IDs, paths, tags, and status, optionally filtered by tags and status.",
	}, h.listMemories)

	return s, nil
}

func Run(ctx context.Context, s *mcp.Server) error {
	return s.Run(ctx, &mcp.StdioTransport{})
}

type semanticSearchInput struct {
	Query string   `json:"query" jsonschema:"required"`
	Tags  []string `json:"tags,omitempty"`
}

type searchResult struct {
	DocID   string   `json:"doc_id"`
	ChunkID string   `json:"chunk_id"`
	Heading string   `json:"heading"`
	Content string   `json:"content"`
	Score   float64  `json:"score"`
	DocPath string   `json:"doc_path"`
	Tags    []string `json:"tags"`
	Status  string   `json:"status"`
}

type semanticSearchOutput struct {
	Results []searchResult `json:"results"`
}

func (s *server) semanticSearch(_ context.Context, _ *mcp.CallToolRequest, in semanticSearchInput) (*mcp.CallToolResult, semanticSearchOutput, error) {
	if s.deps.Embedder == nil || s.deps.Index == nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("index unavailable: embedding model not loaded")
	}
	terms := strings.Fields(in.Query)
	if len(terms) == 0 {
		terms = []string{in.Query}
	}
	expanded := s.deps.Taxonomy.ExpandQuery(terms)
	query := strings.Join(expanded, " ")
	if query == "" {
		query = in.Query
	}
	vec, err := s.deps.Embedder.Embed(query)
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("embed query: %w", err)
	}
	topK := 5
	if s.deps.Config != nil && s.deps.Config.Embeddings.SearchTopK > 0 {
		topK = s.deps.Config.Embeddings.SearchTopK
	}
	results, err := s.deps.Index.Search(vec, in.Tags, topK)
	if err != nil {
		return nil, semanticSearchOutput{}, fmt.Errorf("search: %w", err)
	}
	out := semanticSearchOutput{Results: make([]searchResult, 0, len(results))}
	for _, r := range results {
		hit := searchResult{
			DocID:   r.DocID,
			ChunkID: r.ChunkID,
			Heading: r.Heading,
			Content: r.Content,
			Score:   1 - r.Distance,
		}
		if doc, err := s.deps.Index.GetDoc(r.DocID); err == nil {
			hit.DocPath = doc.Path
			hit.Status = doc.Status
			hit.Tags = parseTagsJSON(doc.TagsJSON)
		}
		out.Results = append(out.Results, hit)
	}
	return nil, out, nil
}

type markdownPatch struct {
	DocID       string         `json:"doc_id" jsonschema:"required"`
	Op          string         `json:"op" jsonschema:"required"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	Body        string         `json:"body,omitempty"`
	Heading     string         `json:"heading,omitempty"`
	Content     string         `json:"content,omitempty"`
}

type appendMemoryInput struct {
	Entry struct {
		Action     string   `json:"action" jsonschema:"required"`
		TargetDocs []string `json:"target_docs" jsonschema:"required"`
		Tags       []string `json:"tags" jsonschema:"required"`
		Summary    string   `json:"summary" jsonschema:"required"`
		Reasoning  string   `json:"reasoning,omitempty"`
	} `json:"entry" jsonschema:"required"`
	MarkdownUpdates []markdownPatch `json:"markdown_updates,omitempty"`
}

type appendMemoryOutput struct {
	LogID string `json:"log_id"`
}

func (s *server) appendMemory(ctx context.Context, _ *mcp.CallToolRequest, in appendMemoryInput) (*mcp.CallToolResult, appendMemoryOutput, error) {
	tags, err := s.deps.Taxonomy.NormalizeTags(in.Entry.Tags)
	if err != nil {
		return nil, appendMemoryOutput{}, err
	}

	entry := logstore.MemoryLogEntry{
		Action:     logstore.ActionType(in.Entry.Action),
		TargetDocs: in.Entry.TargetDocs,
		Tags:       tags,
		Summary:    in.Entry.Summary,
		Reasoning:  in.Entry.Reasoning,
	}
	if err := s.deps.Log.Append(&entry); err != nil {
		return nil, appendMemoryOutput{}, fmt.Errorf("append log: %w", err)
	}

	for _, p := range in.MarkdownUpdates {
		switch p.Op {
		case "create":
			fm, err := yaml.Marshal(p.Frontmatter)
			if err != nil {
				return nil, appendMemoryOutput{}, fmt.Errorf("marshal frontmatter: %w", err)
			}
			content := append(append([]byte("---\n"), fm...), []byte("---\n"+p.Body)...)
			if _, err := s.deps.Docs.Create(docs.Doc{ID: p.DocID}, content); err != nil {
				return nil, appendMemoryOutput{}, fmt.Errorf("create doc %s: %w", p.DocID, err)
			}
		case "append_section":
			if err := s.deps.Docs.AppendSection(p.DocID, p.Heading, p.Content); err != nil {
				return nil, appendMemoryOutput{}, fmt.Errorf("append section to %s: %w", p.DocID, err)
			}
		default:
			return nil, appendMemoryOutput{}, fmt.Errorf("unsupported markdown op %q: only create and append_section are allowed", p.Op)
		}
	}

	if in.Entry.Action == string(logstore.ActionDeprecated) {
		for _, id := range in.Entry.TargetDocs {
			if err := s.deps.Docs.SetStatus(id, "deprecated"); err != nil {
				return nil, appendMemoryOutput{}, fmt.Errorf("deprecate %s: %w", id, err)
			}
		}
	}

	if s.deps.Embedder != nil && s.deps.Syncer != nil {
		if err := s.deps.Syncer.Sync(ctx); err != nil {
			return nil, appendMemoryOutput{}, fmt.Errorf("reindex: %w", err)
		}
	}

	return nil, appendMemoryOutput{LogID: entry.LogID}, nil
}

func parseTagsJSON(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil {
		return nil
	}
	return tags
}

type getMemoryInput struct {
	DocID string `json:"doc_id" jsonschema:"required"`
}

type getMemoryOutput struct {
	DocID   string `json:"doc_id"`
	Content string `json:"content"`
}

func (s *server) getMemory(_ context.Context, _ *mcp.CallToolRequest, in getMemoryInput) (*mcp.CallToolResult, getMemoryOutput, error) {
	content, err := s.deps.Docs.Get(in.DocID)
	if err != nil {
		return nil, getMemoryOutput{}, err
	}
	return nil, getMemoryOutput{DocID: in.DocID, Content: content}, nil
}

type listMemoriesInput struct {
	Tags   []string `json:"tags,omitempty"`
	Status string   `json:"status,omitempty"`
}

type memoryInfo struct {
	DocID  string   `json:"doc_id"`
	Path   string   `json:"path"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"`
}

type listMemoriesOutput struct {
	Memories []memoryInfo `json:"memories"`
}

func (s *server) listMemories(_ context.Context, _ *mcp.CallToolRequest, in listMemoriesInput) (*mcp.CallToolResult, listMemoriesOutput, error) {
	docs, err := s.deps.Docs.List()
	if err != nil {
		return nil, listMemoriesOutput{}, err
	}
	out := listMemoriesOutput{Memories: make([]memoryInfo, 0, len(docs))}
	for _, d := range docs {
		if in.Status != "" && d.Status != in.Status {
			continue
		}
		if !tagsSubset(in.Tags, d.Tags) {
			continue
		}
		out.Memories = append(out.Memories, memoryInfo{
			DocID:  d.ID,
			Path:   d.Path,
			Tags:   d.Tags,
			Status: d.Status,
		})
	}
	return nil, out, nil
}

func tagsSubset(want, have []string) bool {
	haveSet := make(map[string]struct{}, len(have))
	for _, h := range have {
		haveSet[strings.ToLower(h)] = struct{}{}
	}
	for _, w := range want {
		if _, ok := haveSet[strings.ToLower(w)]; !ok {
			return false
		}
	}
	return true
}
