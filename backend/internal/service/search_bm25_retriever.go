package service

import "context"

// bm25ChunkStore is the subset of store.Store needed for BM25 keyword search.
type bm25ChunkStore interface {
	SearchChunksBM25(ctx context.Context, query string, fileIDs []string, limit int) ([]SourceChunk, error)
}

type bm25ChunkRetriever struct {
	Store bm25ChunkStore
}

type bm25ChunkRetrievalOptions struct {
	Query   string
	FileIDs []string
	Limit   int
}

type bm25ChunkRetrievalResult struct {
	Sources []SourceChunk
}

func (r bm25ChunkRetriever) Retrieve(ctx context.Context, opts bm25ChunkRetrievalOptions) (bm25ChunkRetrievalResult, error) {
	if r.Store == nil || opts.Limit <= 0 {
		return bm25ChunkRetrievalResult{}, nil
	}
	sources, err := r.Store.SearchChunksBM25(ctx, opts.Query, opts.FileIDs, opts.Limit)
	if err != nil {
		return bm25ChunkRetrievalResult{}, err
	}
	return bm25ChunkRetrievalResult{Sources: sources}, nil
}

func (s *SearchService) bm25ChunkRetriever() bm25ChunkRetriever {
	if s == nil {
		return bm25ChunkRetriever{}
	}
	return bm25ChunkRetriever{Store: s.store}
}
