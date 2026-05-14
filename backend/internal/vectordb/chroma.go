// Package vectordb provides a client for the Chroma vector database.
// It supports collection lifecycle management, batch upsert with embeddings,
// similarity queries, and deletion through Chroma's v2 REST API.
package vectordb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultChromaBaseURL = "http://chroma:8000"
	chromaBatchSize      = 100
	defaultTenant        = "default_tenant"
	defaultDatabase      = "default_database"
)

// VectorStore is the interface that all vector database backends must implement.
// It abstracts collection management, batch upsert, similarity query, and deletion.
type VectorStore interface {
	EnsureCollection(ctx context.Context, name string) error
	Upsert(ctx context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error
	Query(ctx context.Context, collection string, queryEmbedding []float32, nResults int) (*QueryResult, error)
	Delete(ctx context.Context, collection string, ids []string) error
}

// QueryResult holds the results of a vector similarity query.
// Each field is a slice whose elements correspond positionally.
type QueryResult struct {
	IDs       []string
	Documents []string
	Distances []float32
	Metadatas []map[string]any
}

// ChromaClient is a VectorStore implementation backed by a Chroma server.
// It caches collection name-to-ID mappings to avoid repeated lookups.
type ChromaClient struct {
	BaseURL  string
	Tenant   string
	Database string

	client            *http.Client
	collectionIDCache map[string]string
	cacheMu           sync.RWMutex
}

// NewChroma creates a new ChromaClient connected to the given base URL.
func NewChroma(baseURL string) *ChromaClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultChromaBaseURL
	}
	return &ChromaClient{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Tenant:   defaultTenant,
		Database: defaultDatabase,
		client:   &http.Client{Timeout: 30 * time.Second},

		collectionIDCache: map[string]string{},
	}
}

// EnsureCollection creates the named collection if it doesn't exist.
// It caches the collection ID on success.
func (c *ChromaClient) EnsureCollection(ctx context.Context, name string) error {
	started := time.Now()
	if strings.TrimSpace(name) == "" {
		return errors.New("collection name is required")
	}
	if id := c.cachedCollectionID(name); id != "" {
		log.Printf("level=debug component=vectordb event=ensure_collection_cached collection=%q collection_id=%q", name, id)
		return nil
	}
	log.Printf("level=info component=vectordb event=ensure_collection_begin collection=%q base_url=%s", name, c.BaseURL)
	// Try GET first; 404 means it doesn't exist yet
	path := fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections/%s",
		url.PathEscape(c.Tenant), url.PathEscape(c.Database), url.PathEscape(name))
	var existing chromaCollection
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &existing); err == nil {
		c.cacheCollectionID(name, existing.ID)
		log.Printf("level=info component=vectordb event=ensure_collection_exists collection=%q duration_ms=%d", name, time.Since(started).Milliseconds())
		return nil // already exists
	} else if !isChromaStatus(err, http.StatusNotFound) {
		log.Printf("level=error component=vectordb event=ensure_collection_lookup_failed collection=%q duration_ms=%d err=%q", name, time.Since(started).Milliseconds(), err)
		return fmt.Errorf("get chroma collection %q: %w", name, err)
	}

	// Create
	var created chromaCollection
	body := map[string]any{
		"name": name,
		"metadata": map[string]any{
			"hnsw:space": "cosine",
		},
	}
	createPath := fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections",
		url.PathEscape(c.Tenant), url.PathEscape(c.Database))
	if err := c.doRequest(ctx, http.MethodPost, createPath, body, &created); err != nil {
		if isChromaStatus(err, http.StatusConflict) || strings.Contains(strings.ToLower(err.Error()), "already exists") {
			if id, resolveErr := c.collectionID(ctx, name); resolveErr == nil {
				c.cacheCollectionID(name, id)
			}
			log.Printf("level=info component=vectordb event=ensure_collection_conflict collection=%q duration_ms=%d", name, time.Since(started).Milliseconds())
			return nil
		}
		log.Printf("level=error component=vectordb event=ensure_collection_create_failed collection=%q duration_ms=%d err=%q", name, time.Since(started).Milliseconds(), err)
		return fmt.Errorf("create chroma collection %q: %w", name, err)
	}
	c.cacheCollectionID(name, created.ID)
	log.Printf("level=info component=vectordb event=ensure_collection_created collection=%q collection_id=%q duration_ms=%d", name, created.ID, time.Since(started).Milliseconds())
	return nil
}

// Upsert inserts or updates vectors with their associated documents and metadata.
// It ensures the collection exists, batches the input by chromaBatchSize, and
// validates that all input slices have matching lengths.
func (c *ChromaClient) Upsert(ctx context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error {
	started := time.Now()
	if len(ids) == 0 {
		log.Printf("level=debug component=vectordb event=upsert_skipped collection=%q reason=empty_input", collection)
		return nil
	}
	if err := validateUpsertInput(ids, embeddings, documents, metadatas); err != nil {
		log.Printf("level=error component=vectordb event=upsert_invalid_input collection=%q ids=%d err=%q", collection, len(ids), err)
		return err
	}
	if metadatas == nil {
		metadatas = make([]map[string]any, len(ids))
	}
	for i := range metadatas {
		if metadatas[i] == nil {
			metadatas[i] = map[string]any{}
		}
	}
	if err := c.EnsureCollection(ctx, collection); err != nil {
		log.Printf("level=error component=vectordb event=upsert_collection_ensure_failed collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), err)
		return err
	}
	collectionID, err := c.collectionID(ctx, collection)
	if err != nil {
		log.Printf("level=error component=vectordb event=upsert_collection_resolve_failed collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), err)
		return err
	}
	upsertPath := fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections/%s/upsert",
		url.PathEscape(c.Tenant), url.PathEscape(c.Database), url.PathEscape(collectionID))
	log.Printf("level=info component=vectordb event=upsert_begin collection=%q ids=%d batch_size=%d", collection, len(ids), chromaBatchSize)
	for start := 0; start < len(ids); start += chromaBatchSize {
		end := start + chromaBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		body := map[string]any{
			"ids":        ids[start:end],
			"embeddings": embeddings[start:end],
			"documents":  documents[start:end],
			"metadatas":  metadatas[start:end],
		}
		if err := c.doRequest(ctx, http.MethodPost, upsertPath, body, nil); err != nil {
			log.Printf("level=error component=vectordb event=upsert_batch_failed collection=%q batch_start=%d batch_end=%d duration_ms=%d err=%q", collection, start, end, time.Since(started).Milliseconds(), err)
			return err
		}
		log.Printf("level=debug component=vectordb event=upsert_batch_complete collection=%q batch_start=%d batch_end=%d", collection, start, end)
	}
	log.Printf("level=info component=vectordb event=upsert_complete collection=%q ids=%d duration_ms=%d", collection, len(ids), time.Since(started).Milliseconds())
	return nil
}

// Query performs a similarity search against the collection using the provided
// query embedding. It returns up to nResults matching documents.
func (c *ChromaClient) Query(ctx context.Context, collection string, queryEmbedding []float32, nResults int) (*QueryResult, error) {
	started := time.Now()
	if len(queryEmbedding) == 0 {
		return nil, errors.New("query embedding is required")
	}
	if nResults <= 0 {
		nResults = 5
	}
	body := map[string]any{
		"query_embeddings": [][]float32{queryEmbedding},
		"n_results":        nResults,
		"include":          []string{"documents", "metadatas", "distances"},
	}
	var response chromaQueryResponse
	collectionID, err := c.collectionID(ctx, collection)
	if err != nil {
		log.Printf("level=error component=vectordb event=query_collection_resolve_failed collection=%q duration_ms=%d err=%q", collection, time.Since(started).Milliseconds(), err)
		return nil, err
	}
	path := fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections/%s/query",
		url.PathEscape(c.Tenant), url.PathEscape(c.Database), url.PathEscape(collectionID))
	log.Printf("level=info component=vectordb event=query_begin collection=%q n_results=%d embedding_dimensions=%d", collection, nResults, len(queryEmbedding))
	if err := c.doRequest(ctx, http.MethodPost, path, body, &response); err != nil {
		log.Printf("level=error component=vectordb event=query_failed collection=%q duration_ms=%d err=%q", collection, time.Since(started).Milliseconds(), err)
		return nil, err
	}
	result := response.firstResult()
	log.Printf("level=info component=vectordb event=query_complete collection=%q results=%d duration_ms=%d", collection, len(result.IDs), time.Since(started).Milliseconds())
	return result, nil
}

// Delete removes vectors by ID from the collection.
// After deletion, it refreshes the collection cache and ensures the collection exists.
func (c *ChromaClient) Delete(ctx context.Context, collection string, ids []string) error {
	started := time.Now()
	if len(ids) == 0 {
		log.Printf("level=debug component=vectordb event=delete_skipped collection=%q reason=empty_input", collection)
		return nil
	}
	collectionID, err := c.collectionID(ctx, collection)
	if err != nil {
		log.Printf("level=error component=vectordb event=delete_collection_resolve_failed collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), err)
		return err
	}
	body := map[string]any{"ids": ids}
	path := fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections/%s/delete",
		url.PathEscape(c.Tenant), url.PathEscape(c.Database), url.PathEscape(collectionID))
	log.Printf("level=info component=vectordb event=delete_begin collection=%q ids=%d", collection, len(ids))
	if err := c.doRequest(ctx, http.MethodPost, path, body, nil); err != nil {
		if isChromaStatus(err, http.StatusNotFound) {
			c.invalidateCollectionID(collection)
			ensureErr := c.EnsureCollection(ctx, collection)
			if ensureErr != nil {
				log.Printf("level=error component=vectordb event=delete_collection_restore_failed collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), ensureErr)
				return ensureErr
			}
			log.Printf("level=warn component=vectordb event=delete_collection_missing collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), err)
			return nil
		}
		log.Printf("level=error component=vectordb event=delete_failed collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), err)
		return err
	}
	c.invalidateCollectionID(collection)
	if err := c.EnsureCollection(ctx, collection); err != nil {
		log.Printf("level=error component=vectordb event=delete_collection_restore_failed collection=%q ids=%d duration_ms=%d err=%q", collection, len(ids), time.Since(started).Milliseconds(), err)
		return err
	}
	log.Printf("level=info component=vectordb event=delete_complete collection=%q ids=%d duration_ms=%d", collection, len(ids), time.Since(started).Milliseconds())
	return nil
}

func (c *ChromaClient) collectionID(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("collection name is required")
	}
	if id := c.cachedCollectionID(name); id != "" {
		return id, nil
	}

	path := fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections/%s",
		url.PathEscape(c.Tenant), url.PathEscape(c.Database), url.PathEscape(name))
	var collection chromaCollection
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &collection); err != nil {
		if isChromaStatus(err, http.StatusNotFound) {
			if ensureErr := c.EnsureCollection(ctx, name); ensureErr != nil {
				return "", fmt.Errorf("ensure chroma collection %q: %w", name, ensureErr)
			}
			if id := c.cachedCollectionID(name); id != "" {
				return id, nil
			}
		}
		return "", fmt.Errorf("resolve chroma collection %q: %w", name, err)
	}
	if strings.TrimSpace(collection.ID) == "" {
		return "", fmt.Errorf("resolve chroma collection %q: empty collection id", name)
	}
	c.cacheCollectionID(name, collection.ID)
	return collection.ID, nil
}

func (c *ChromaClient) cachedCollectionID(name string) string {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.collectionIDCache[strings.TrimSpace(name)]
}

func (c *ChromaClient) cacheCollectionID(name, id string) {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name == "" || id == "" {
		return
	}
	c.cacheMu.Lock()
	c.collectionIDCache[name] = id
	c.cacheMu.Unlock()
}

func (c *ChromaClient) invalidateCollectionID(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	c.cacheMu.Lock()
	delete(c.collectionIDCache, name)
	c.cacheMu.Unlock()
}

func (c *ChromaClient) doRequest(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/"+strings.TrimLeft(path, "/"), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("chroma request failed (%s %s): %w", method, path, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("read chroma response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &chromaHTTPError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(responseBody)),
		}
	}
	if out == nil || len(responseBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode chroma response (%s %s): %w", method, path, err)
	}
	return nil
}

func validateUpsertInput(ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error {
	count := len(ids)
	if len(embeddings) != count {
		return fmt.Errorf("embeddings length %d does not match ids length %d", len(embeddings), count)
	}
	if len(documents) != count {
		return fmt.Errorf("documents length %d does not match ids length %d", len(documents), count)
	}
	if metadatas != nil && len(metadatas) != count {
		return fmt.Errorf("metadatas length %d does not match ids length %d", len(metadatas), count)
	}
	for i, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("ids[%d] is empty", i)
		}
		if len(embeddings[i]) == 0 {
			return fmt.Errorf("embeddings[%d] is empty", i)
		}
	}
	return nil
}

type chromaCollection struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
}

type chromaQueryResponse struct {
	IDs       [][]string         `json:"ids"`
	Documents [][]string         `json:"documents"`
	Distances [][]float32        `json:"distances"`
	Metadatas [][]map[string]any `json:"metadatas"`
}

func (r chromaQueryResponse) firstResult() *QueryResult {
	result := &QueryResult{}
	if len(r.IDs) > 0 {
		result.IDs = r.IDs[0]
	}
	if len(r.Documents) > 0 {
		result.Documents = r.Documents[0]
	}
	if len(r.Distances) > 0 {
		result.Distances = r.Distances[0]
	}
	if len(r.Metadatas) > 0 {
		result.Metadatas = r.Metadatas[0]
	}
	return result
}

type chromaHTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *chromaHTTPError) Error() string {
	body := e.Body
	if body == "" {
		body = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("chroma %s %s failed (status: %d): %s", e.Method, e.Path, e.StatusCode, body)
}

func isChromaStatus(err error, status int) bool {
	var httpErr *chromaHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == status
}
