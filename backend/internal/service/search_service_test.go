package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

type mockSearchProvider struct {
	mu             sync.Mutex
	embedErr       error
	embeddings     [][]float32
	embedCalls     int
	embedTexts     []string
	chatCalled     bool
	chatErr        error
	chatMsgs       []llm.Message
	completeResult string
	completeCalls  int
}

func (p *mockSearchProvider) Name() string {
	return "mock"
}

func (p *mockSearchProvider) Chat(ctx context.Context, messages []llm.Message) (<-chan string, error) {
	p.chatCalled = true
	p.chatMsgs = append([]llm.Message(nil), messages...)
	if p.chatErr != nil {
		return nil, p.chatErr
	}
	ch := make(chan string, 1)
	ch <- "ok"
	close(ch)
	return ch, nil
}

func (p *mockSearchProvider) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completeCalls++
	if p.completeResult != "" {
		return p.completeResult, nil
	}
	return "", nil
}

func (p *mockSearchProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.embedCalls++
	p.embedTexts = append(p.embedTexts, texts...)
	if p.embedErr != nil {
		return nil, p.embedErr
	}
	if p.embeddings != nil {
		return p.embeddings, nil
	}
	return [][]float32{{0.1, 0.2, 0.3}}, nil
}

type mockVectorStore struct {
	queryResult   *vectordb.QueryResult
	queryErr      error
	queryNResults int
	queryCalled   bool
	queryCalls    int
}

func (s *mockVectorStore) EnsureCollection(ctx context.Context, name string) error {
	return nil
}

func (s *mockVectorStore) Upsert(ctx context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error {
	return nil
}

func (s *mockVectorStore) Query(ctx context.Context, collection string, queryEmbedding []float32, nResults int) (*vectordb.QueryResult, error) {
	s.queryCalled = true
	s.queryCalls++
	s.queryNResults = nResults
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	return s.queryResult, nil
}

func (s *mockVectorStore) Delete(ctx context.Context, collection string, ids []string) error {
	return nil
}

func TestSearchReturnsSourceChunks(t *testing.T) {
	vector := &mockVectorStore{queryResult: sampleQueryResult()}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 2}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "login issue"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(response.Results))
	}
	if response.Results[0].FileID != "file-a" || response.Results[0].FileName != "Guide.md" {
		t.Fatalf("unexpected first source: %#v", response.Results[0])
	}
	if response.Results[0].Score < 0.89 || response.Results[0].Score > 0.91 {
		t.Fatalf("expected raw cosine similarity score ~0.90, got %f", response.Results[0].Score)
	}
	if vector.queryNResults != 2 {
		t.Fatalf("expected query topK 2, got %d", vector.queryNResults)
	}
}

func TestSearchReadsChunkMetadataContract(t *testing.T) {
	parentID := indexing.ParentChunkID("file-contract", 1)
	vector := &mockVectorStore{queryResult: &vectordb.QueryResult{
		IDs:       []string{indexing.ChunkID("file-contract", 7)},
		Documents: []string{"Contract body"},
		Distances: []float32{0.25},
		Metadatas: []map[string]any{
			(indexing.ChunkMetadata{
				FileID:        "file-contract",
				FileName:      "Contract.md",
				Heading:       "Terms",
				ChunkIndex:    7,
				Source:        "markdown",
				ParentChunkID: parentID,
			}).Map(),
		},
	}}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 1}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "contract terms"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("expected one result, got %#v", response.Results)
	}
	got := response.Results[0]
	if got.ID != indexing.ChunkID("file-contract", 7) ||
		got.FileID != "file-contract" ||
		got.FileName != "Contract.md" ||
		got.Heading != "Terms" ||
		got.ChunkIndex != 7 ||
		got.ParentID != parentID {
		t.Fatalf("metadata contract was not preserved in source chunk: %#v", got)
	}
	if got.Score < 0.749 || got.Score > 0.751 {
		t.Fatalf("expected score from vector distance, got %.4f", got.Score)
	}
}

func TestSearchFiltersByFileIDsAndRequestsCandidates(t *testing.T) {
	vector := &mockVectorStore{queryResult: sampleQueryResult()}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 2}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "login issue", FileIDs: []string{"file-b"}, TopK: 1})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].FileID != "file-b" {
		t.Fatalf("expected one filtered file-b result, got %#v", response.Results)
	}
	if vector.queryNResults != minCandidateTopK {
		t.Fatalf("expected candidate query limit %d, got %d", minCandidateTopK, vector.queryNResults)
	}
}

func TestSearchIntentPrefiltersSemanticResults(t *testing.T) {
	db := newSearchServiceStore(t)
	report := &model.File{ID: "report", Name: "季报2024.pdf", Path: "/", StoragePath: "report.pdf", Size: 200, MimeType: "application/pdf", Status: model.FileStatusReady}
	note := &model.File{ID: "note", Name: "奶茶店清单.md", Path: "/", StoragePath: "note.md", Size: 100, MimeType: "text/markdown", Status: model.FileStatusReady}
	for _, file := range []*model.File{report, note} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	vector := &mockVectorStore{queryResult: &vectordb.QueryResult{
		IDs:       []string{"report#0", "note#0"},
		Documents: []string{"季度报告内容", "奶茶店选址"},
		Distances: []float32{0.1, 0.2},
		Metadatas: []map[string]any{
			{"file_id": "report", "file_name": "季报2024.pdf", "chunk_index": 0},
			{"file_id": "note", "file_name": "奶茶店清单.md", "chunk_index": 0},
		},
	}}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5, IntentParse: true, IntentFileLimit: 500}}, db, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "pdf"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if response.Intent == nil || len(response.Intent.Extensions) != 1 || response.Intent.Extensions[0] != "pdf" {
		t.Fatalf("expected pdf intent, got %#v", response.Intent)
	}
	if len(response.Results) != 1 || response.Results[0].FileID != "report" {
		t.Fatalf("expected semantic results constrained to report, got %#v", response.Results)
	}
}

func TestSearchIntentEmptyFilterReturnsEmptyResults(t *testing.T) {
	db := newSearchServiceStore(t)
	file := &model.File{ID: "report", Name: "季报2024.pdf", Path: "/", StoragePath: "report.pdf", Size: 200, MimeType: "application/pdf", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	vector := &mockVectorStore{}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5, IntentParse: true, IntentFileLimit: 500}}, db, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "pptx"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if response.Results == nil {
		t.Fatal("expected empty results slice, got nil")
	}
	if len(response.Results) != 0 {
		t.Fatalf("expected no results, got %#v", response.Results)
	}
	if vector.queryCalled {
		t.Fatal("expected intent prefilter to skip vector search when no files match")
	}
}

func TestSearchAppliesMinScore(t *testing.T) {
	vector := &mockVectorStore{queryResult: sampleQueryResult()}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 3, MinScore: 0.85}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "login issue"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].FileID != "file-a" {
		t.Fatalf("expected only high score result, got %#v", response.Results)
	}
}

func TestSearchAppliesScorePercentile(t *testing.T) {
	vector := &mockVectorStore{queryResult: &vectordb.QueryResult{
		IDs:       []string{"a#0", "b#0", "c#0", "d#0"},
		Documents: []string{"a", "b", "c", "d"},
		Distances: []float32{0.1, 0.2, 0.7, 0.8},
		Metadatas: []map[string]any{
			{"file_id": "a", "file_name": "A.md", "chunk_index": 0},
			{"file_id": "b", "file_name": "B.md", "chunk_index": 0},
			{"file_id": "c", "file_name": "C.md", "chunk_index": 0},
			{"file_id": "d", "file_name": "D.md", "chunk_index": 0},
		},
	}}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 4, ScorePercentile: 0.5}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "content"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("expected top half after percentile filter, got %#v", response.Results)
	}
	if response.Results[0].ID != "a#0" || response.Results[1].ID != "b#0" {
		t.Fatalf("unexpected percentile results: %#v", response.Results)
	}
}

func TestSearchExpandsQueries(t *testing.T) {
	provider := &mockSearchProvider{completeResult: "1. contract amount\n2. agreement price\ncontract amount"}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{MultiQuery: true, MultiQueryCount: 2}}, nil, provider, &mockVectorStore{})

	queries := service.expandQueries(context.Background(), "合同金额", 2)
	if provider.completeCalls != 1 {
		t.Fatalf("expected one Complete call, got %d", provider.completeCalls)
	}
	if len(queries) != 3 {
		t.Fatalf("expected original plus two variants, got %#v", queries)
	}
	if queries[0] != "合同金额" || queries[1] != "contract amount" || queries[2] != "agreement price" {
		t.Fatalf("unexpected query expansion: %#v", queries)
	}
}

func TestSearchUsesMultiQueryExpansionThroughSearchInterface(t *testing.T) {
	provider := &mockSearchProvider{completeResult: "1. contract amount\n2. agreement price"}
	vector := &mockVectorStore{queryResult: sampleQueryResult()}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 2, MultiQuery: true, MultiQueryCount: 2}}, nil, provider, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "合同金额"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if provider.completeCalls != 1 {
		t.Fatalf("expected one query expansion call, got %d", provider.completeCalls)
	}
	if vector.queryCalls != 3 {
		t.Fatalf("expected vector query for original plus two variants, got %d", vector.queryCalls)
	}
	if len(response.Results) != 2 {
		t.Fatalf("expected deduped results from expanded queries, got %#v", response.Results)
	}
	if response.Results[0].FileID != "file-a" || response.Results[1].FileID != "file-b" {
		t.Fatalf("unexpected merged results: %#v", response.Results)
	}
}

func TestHybridSearchFallsBackToChunkStoreAndParentText(t *testing.T) {
	db := newSearchServiceStore(t)
	file := &model.File{ID: "code-file", Name: "api.md", Path: "/", StoragePath: "api.md", Size: 100, MimeType: "text/markdown", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	parentID := indexing.ParentChunkID(file.ID, 0)
	if err := db.UpsertChunks(context.Background(), []store.ChunkRow{
		{ID: parentID, FileID: file.ID, FileName: file.Name, Heading: "API", ChunkIndex: 0, Text: "## API\n完整上下文 uniqueFunc 包含参数说明", IsParent: true},
		{ID: indexing.ChunkID(file.ID, 0), FileID: file.ID, FileName: file.Name, Heading: "API", ChunkIndex: 0, Text: "uniqueFunc", ParentChunkID: parentID},
	}); err != nil {
		t.Fatalf("upsert chunks: %v", err)
	}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 3, HybridSearch: true, RRFConstant: 60}}, db, &mockSearchProvider{}, &mockVectorStore{queryResult: &vectordb.QueryResult{}})

	response, err := service.Search(context.Background(), SearchRequest{Query: "uniqueFunc"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("expected one BM25 fallback result, got %#v", response.Results)
	}
	if response.Results[0].ID != indexing.ChunkID(file.ID, 0) {
		t.Fatalf("unexpected result id: %#v", response.Results[0])
	}
	if !strings.Contains(response.Results[0].Text, "完整上下文") {
		t.Fatalf("expected parent text to be resolved, got %q", response.Results[0].Text)
	}
}

func TestHybridSearchUsesBM25WhenVectorDependenciesUnavailable(t *testing.T) {
	db := newSearchServiceStore(t)
	file := &model.File{ID: "notes", Name: "notes.md", Path: "/", StoragePath: "notes.md", Size: 100, MimeType: "text/markdown", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	parentID := indexing.ParentChunkID(file.ID, 0)
	if err := db.UpsertChunks(context.Background(), []store.ChunkRow{
		{ID: parentID, FileID: file.ID, FileName: file.Name, Heading: "Notes", ChunkIndex: 0, Text: "## Notes\n完整上下文 localOnly 包含离线搜索内容", IsParent: true},
		{ID: indexing.ChunkID(file.ID, 0), FileID: file.ID, FileName: file.Name, Heading: "Notes", ChunkIndex: 0, Text: "localOnly", ParentChunkID: parentID},
	}); err != nil {
		t.Fatalf("upsert chunks: %v", err)
	}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 3, HybridSearch: true}}, db, nil, nil)

	response, err := service.Search(context.Background(), SearchRequest{Query: "localOnly"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("expected one BM25 result, got %#v", response.Results)
	}
	if response.Results[0].ID != indexing.ChunkID(file.ID, 0) {
		t.Fatalf("unexpected result id: %#v", response.Results[0])
	}
	if !strings.Contains(response.Results[0].Text, "完整上下文") {
		t.Fatalf("expected parent text to be resolved, got %q", response.Results[0].Text)
	}
}

func TestSearchReturnsUnavailableWhenNoRetrievalBackendIsConfigured(t *testing.T) {
	service := NewSearchService(&config.Config{}, nil, nil, nil)

	_, err := service.Search(context.Background(), SearchRequest{Query: "content"})
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("expected ErrServiceUnavailable, got %v", err)
	}
}

func TestSearchRejectsEmptyQuery(t *testing.T) {
	service := NewSearchService(&config.Config{}, nil, &mockSearchProvider{}, &mockVectorStore{})

	_, err := service.Search(context.Background(), SearchRequest{Query: "  "})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func TestSearchHandlesMissingMetadata(t *testing.T) {
	vector := &mockVectorStore{queryResult: &vectordb.QueryResult{
		IDs:       []string{"chunk-1"},
		Documents: []string{"some content"},
		Distances: []float32{0.2},
	}}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 1}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "content"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(response.Results))
	}
	if response.Results[0].ChunkIndex != -1 {
		t.Fatalf("expected missing chunk index fallback -1, got %d", response.Results[0].ChunkIndex)
	}
}

func TestSearchRejectsEmptyEmbedding(t *testing.T) {
	service := NewSearchService(&config.Config{}, nil, &mockSearchProvider{embeddings: [][]float32{}}, &mockVectorStore{})

	_, err := service.Search(context.Background(), SearchRequest{Query: "content"})
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}

func TestSearchFilesReturnsMetadataHit(t *testing.T) {
	db := newSearchServiceStore(t)
	seedSearchServiceFiles(t, db)
	service := NewSearchService(&config.Config{}, db, &mockSearchProvider{}, &mockVectorStore{})

	response, err := service.SearchFiles(context.Background(), FileSearchRequest{Query: "A7M3"})
	if err != nil {
		t.Fatalf("SearchFiles returned error: %v", err)
	}
	if response.Total != 1 {
		t.Fatalf("expected one hit, got %d", response.Total)
	}
	hit := response.Hits[0]
	if hit.File.ID != "image-a" || len(hit.MatchTypes) != 1 || hit.MatchTypes[0] != "meta" {
		t.Fatalf("expected metadata image hit, got %#v", hit)
	}
	if hit.Score < 0.699 || hit.Score > 0.701 {
		t.Fatalf("expected metadata score 0.7, got %f", hit.Score)
	}
}

func TestSearchFilesNameHitDoesNotCallSemanticWhenDisabled(t *testing.T) {
	db := newSearchServiceStore(t)
	seedSearchServiceFiles(t, db)
	provider := &mockSearchProvider{}
	service := NewSearchService(&config.Config{}, db, provider, &mockVectorStore{queryResult: sampleFileSearchQueryResult()})

	response, err := service.SearchFiles(context.Background(), FileSearchRequest{Query: "奶茶", Semantic: false})
	if err != nil {
		t.Fatalf("SearchFiles returned error: %v", err)
	}
	if provider.embedCalls != 0 {
		t.Fatalf("expected semantic disabled to skip embed, got %d embed calls", provider.embedCalls)
	}
	if response.Total != 1 || response.Hits[0].File.ID != "milk-tea" {
		t.Fatalf("expected milk tea name hit, got %#v", response.Hits)
	}
	if response.Hits[0].Score != 1 {
		t.Fatalf("expected name score 1, got %f", response.Hits[0].Score)
	}
}

func TestSearchFilesMergesSemanticHit(t *testing.T) {
	db := newSearchServiceStore(t)
	seedSearchServiceFiles(t, db)
	provider := &mockSearchProvider{}
	vector := &mockVectorStore{queryResult: sampleFileSearchQueryResult()}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, db, provider, vector)

	response, err := service.SearchFiles(context.Background(), FileSearchRequest{Query: "奶茶", Semantic: true})
	if err != nil {
		t.Fatalf("SearchFiles returned error: %v", err)
	}
	if !response.Semantic {
		t.Fatal("expected semantic to be enabled")
	}
	if provider.embedCalls == 0 || !vector.queryCalled {
		t.Fatal("expected semantic search to call embed and vector query")
	}
	if response.Total != 1 {
		t.Fatalf("expected one merged hit, got %d", response.Total)
	}
	hit := response.Hits[0]
	if len(hit.MatchTypes) != 2 || hit.MatchTypes[0] != "name" || hit.MatchTypes[1] != "semantic" {
		t.Fatalf("expected name+semantic match types, got %#v", hit.MatchTypes)
	}
	if hit.Score != 1 {
		t.Fatalf("expected capped score 1, got %f", hit.Score)
	}
}

func TestSearchFilesIntentOnlyFilterReturnsFiles(t *testing.T) {
	db := newSearchServiceStore(t)
	seedSearchServiceFiles(t, db)
	report := &model.File{ID: "report", Name: "季报2024.pdf", Path: "/", StoragePath: "report.pdf", Size: 200, MimeType: "application/pdf", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{IntentParse: true, IntentFileLimit: 500}}, db, &mockSearchProvider{}, &mockVectorStore{})

	response, err := service.SearchFiles(context.Background(), FileSearchRequest{Query: "pdf"})
	if err != nil {
		t.Fatalf("SearchFiles returned error: %v", err)
	}
	if response.Intent == nil || len(response.Intent.Extensions) != 1 || response.Intent.Extensions[0] != "pdf" {
		t.Fatalf("expected pdf intent, got %#v", response.Intent)
	}
	if response.Total != 1 || response.Hits[0].File.ID != "report" {
		t.Fatalf("expected report filter hit, got %#v", response.Hits)
	}
	if len(response.Hits[0].MatchTypes) != 1 || response.Hits[0].MatchTypes[0] != "filter" {
		t.Fatalf("expected filter match type, got %#v", response.Hits[0].MatchTypes)
	}
}

func TestSearchFilesRejectsEmptyQuery(t *testing.T) {
	db := newSearchServiceStore(t)
	service := NewSearchService(&config.Config{}, db, &mockSearchProvider{}, &mockVectorStore{})

	_, err := service.SearchFiles(context.Background(), FileSearchRequest{Query: "  "})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected ErrEmptyQuery, got %v", err)
	}
}

func sampleQueryResult() *vectordb.QueryResult {
	return &vectordb.QueryResult{
		IDs:       []string{"file-a#0", "file-b#1"},
		Documents: []string{"Guide login troubleshooting", "Roadmap search content"},
		Distances: []float32{0.1, 0.3},
		Metadatas: []map[string]any{
			{"file_id": "file-a", "file_name": "Guide.md", "heading": "Login", "chunk_index": float64(0)},
			{"file_id": "file-b", "file_name": "Roadmap.md", "heading": "Search", "chunk_index": 1},
		},
	}
}

func sampleFileSearchQueryResult() *vectordb.QueryResult {
	return &vectordb.QueryResult{
		IDs:       []string{"milk-tea#0"},
		Documents: []string{"奶茶店选址和菜单规划"},
		Distances: []float32{0.2},
		Metadatas: []map[string]any{
			{"file_id": "milk-tea", "file_name": "奶茶店清单.md", "heading": "门店", "chunk_index": 0},
		},
	}
}

func newSearchServiceStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "memodrive.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := store.Open(context.Background(), &config.Config{Storage: config.StorageConfig{DBPath: dbPath}})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func seedSearchServiceFiles(t *testing.T, db *store.Store) {
	t.Helper()
	files := []*model.File{
		{ID: "image-a", Name: "图片A.jpg", Path: "/Photos", StoragePath: "image-a.jpg", Size: 100, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "milk-tea", Name: "奶茶店清单.md", Path: "/Notes", StoragePath: "milk-tea.md", Size: 300, MimeType: "text/markdown", Status: model.FileStatusReady},
	}
	for _, file := range files {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{
		FileID:   "image-a",
		MetaJSON: `{"camera":"Sony A7M3","format":"JPEG"}`,
	}); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}
}

// Regression: single low-quality result should NOT get score=1.0.
// normalizeScores used to force every top result to 1.0 regardless of actual similarity.
func TestSearchPreservesRawSimilarityScoreForSingleResult(t *testing.T) {
	vector := &mockVectorStore{queryResult: &vectordb.QueryResult{
		IDs:       []string{"bad#0"},
		Documents: []string{"unrelated content"},
		Distances: []float32{0.85},
		Metadatas: []map[string]any{
			{"file_id": "bad", "file_name": "unrelated.md", "chunk_index": 0},
		},
	}}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "relevant query"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(response.Results))
	}
	score := response.Results[0].Score
	if score > 0.9 {
		t.Fatalf("low-quality result (raw similarity ~0.15) should NOT have score %.4f > 0.9", score)
	}
	if score < 0.10 || score > 0.20 {
		t.Fatalf("expected score near raw cosine similarity 0.15, got %.4f", score)
	}
}

// Regression: scores should be differentiated after search, not all clamped to 1.0.
func TestSearchScoresAreDifferentiated(t *testing.T) {
	vector := &mockVectorStore{queryResult: &vectordb.QueryResult{
		IDs:       []string{"good#0", "poor#1"},
		Documents: []string{"relevant content", "unrelated content"},
		Distances: []float32{0.1, 0.8},
		Metadatas: []map[string]any{
			{"file_id": "good", "file_name": "relevant.md", "chunk_index": 0},
			{"file_id": "poor", "file_name": "unrelated.md", "chunk_index": 1},
		},
	}}
	service := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, nil, &mockSearchProvider{}, vector)

	response, err := service.Search(context.Background(), SearchRequest{Query: "relevant"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(response.Results))
	}
	goodScore := response.Results[0].Score
	poorScore := response.Results[1].Score
	if goodScore-poorScore < 0.3 {
		t.Fatalf("scores should be differentiated: good=%.4f poor=%.4f (diff=%.4f)", goodScore, poorScore, goodScore-poorScore)
	}
	if goodScore > 0.95 {
		t.Fatalf("good score %.4f should not be inflated to near 1.0", goodScore)
	}
}
