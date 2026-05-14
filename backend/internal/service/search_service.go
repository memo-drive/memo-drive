package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

var (
	ErrEmptyQuery         = errors.New("query is required")
	ErrServiceUnavailable = errors.New("service is unavailable")
)

const (
	defaultRAGTopK         = 5
	defaultSearchTopK      = 10
	defaultMultiQueryCount = 3
	defaultRRFK            = 60
	maxRAGTopK             = 50
	maxSearchTopK          = 100
	defaultSnippetLength   = 240
	minCandidateTopK       = 20
	defaultFileLimit       = 50
	maxFileLimit           = 200
)

const multiQueryPrompt = `你是个人云盘 MemoDrive 的搜索查询扩展器。用户的文件包括文档、笔记、表格、演示文稿、图片和音频转录等。

任务：为下面的搜索查询生成 %d 个补充查询，最大化向量检索的召回多样性。

扩展策略（每个变体侧重不同策略）：
- 同义替换：用近义词或不同表达方式改写
- 粒度调整：更具体或更概括的说法
- 文档视角：用文档标题、章节标题中常见的表述方式
- 跨语言：如果原查询是中文，可生成对应的英文关键短语，反之亦然

约束：
- 每行一个查询，不编号、不解释、不重复原始查询
- 每个查询控制在 5-25 字（或等量英文单词）
- 只输出查询文本

原始查询：%s`

type SearchService struct {
	cfg      *config.Config
	store    *store.Store
	llm      llm.Provider
	vectorDB vectordb.VectorStore
}

func NewSearchService(cfg *config.Config, db *store.Store, llmProvider llm.Provider, vectorDB vectordb.VectorStore) *SearchService {
	return &SearchService{
		cfg:      cfg,
		store:    db,
		llm:      llmProvider,
		vectorDB: vectorDB,
	}
}

func (s *SearchService) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	started := time.Now()
	plan, earlyResponse, err := s.buildChunkRetrievalPlan(ctx, req, started)
	if err != nil {
		return nil, err
	}
	if earlyResponse != nil {
		return earlyResponse, nil
	}
	if !plan.Retrieval.Available() {
		return nil, fmt.Errorf("%w: no chunk retrieval backend is configured", ErrServiceUnavailable)
	}
	log.Printf("level=info component=search event=search_begin query_chars=%d top_k=%d file_filter=%d provider=%s queries=%d vector=%t bm25=%t hybrid=%t",
		len([]rune(plan.Query)), plan.TopK, len(plan.FileIDs), s.searchProviderName(), len(plan.Queries), plan.Retrieval.Vector, plan.Retrieval.BM25, s.hybridSearch())

	sources, err := s.retrieveChunkEvidence(ctx, plan)
	if err != nil {
		log.Printf("level=error component=search event=search_failed query_chars=%d duration_ms=%d err=%q", len([]rune(plan.Query)), time.Since(started).Milliseconds(), err)
		return nil, err
	}
	if sources == nil {
		sources = []SourceChunk{}
	}
	logScoreDistribution("search", sources)
	log.Printf("level=info component=search event=search_complete results=%d queries=%d duration_ms=%d",
		len(sources), len(plan.Queries), time.Since(started).Milliseconds())

	return &SearchResponse{
		Query:   plan.Query,
		Results: sources,
		Intent:  plan.Intent,
	}, nil
}

func (s *SearchService) searchProviderName() string {
	if s == nil || s.llm == nil {
		return "none"
	}
	return s.llm.Name()
}
