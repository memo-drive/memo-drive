package vectordb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestChromaEnsureCollectionCreatesAndCaches(t *testing.T) {
	var getCount atomic.Int32
	var upsertCount atomic.Int32
	collectionPath := "/api/v2/tenants/default_tenant/databases/default_database/collections/memodrive"
	collectionIDPath := "/api/v2/tenants/default_tenant/databases/default_database/collections/collection-1"
	collectionsPath := "/api/v2/tenants/default_tenant/databases/default_database/collections"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == collectionPath:
			getCount.Add(1)
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == collectionsPath:
			var request struct {
				Name     string         `json:"name"`
				Metadata map[string]any `json:"metadata"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if request.Name != "memodrive" || request.Metadata["hnsw:space"] != "cosine" {
				t.Fatalf("unexpected create request: %#v", request)
			}
			_ = json.NewEncoder(w).Encode(chromaCollection{ID: "collection-1", Name: "memodrive"})
		case r.Method == http.MethodPost && r.URL.Path == collectionIDPath+"/upsert":
			upsertCount.Add(1)
			var request struct {
				IDs []string `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode upsert request: %v", err)
			}
			if len(request.IDs) == 0 || len(request.IDs) > chromaBatchSize {
				t.Fatalf("unexpected upsert batch size: %d", len(request.IDs))
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewChroma(server.URL)
	if err := client.EnsureCollection(context.Background(), "memodrive"); err != nil {
		t.Fatalf("ensure collection failed: %v", err)
	}

	ids := make([]string, chromaBatchSize+1)
	embeddings := make([][]float32, chromaBatchSize+1)
	documents := make([]string, chromaBatchSize+1)
	metadatas := make([]map[string]any, chromaBatchSize+1)
	for i := range ids {
		ids[i] = "id-" + strconv.Itoa(i)
		embeddings[i] = []float32{float32(i)}
		documents[i] = "doc-" + strconv.Itoa(i)
		metadatas[i] = map[string]any{"index": i}
	}
	if err := client.Upsert(context.Background(), "memodrive", ids, embeddings, documents, metadatas); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if getCount.Load() != 1 {
		t.Fatalf("expected one collection lookup, got %d lookups", getCount.Load())
	}
	if upsertCount.Load() != 2 {
		t.Fatalf("expected 2 upsert batches, got %d", upsertCount.Load())
	}
}

func TestChromaQueryAndDelete(t *testing.T) {
	collectionPath := "/api/v2/tenants/default_tenant/databases/default_database/collections/memodrive"
	collectionIDPath := "/api/v2/tenants/default_tenant/databases/default_database/collections/collection-1"
	var getCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == collectionPath:
			getCount.Add(1)
			_ = json.NewEncoder(w).Encode(chromaCollection{ID: "collection-1", Name: "memodrive"})
		case r.Method == http.MethodPost && r.URL.Path == collectionIDPath+"/query":
			var request struct {
				QueryEmbeddings [][]float32 `json:"query_embeddings"`
				NResults        int         `json:"n_results"`
				Include         []string    `json:"include"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode query request: %v", err)
			}
			if request.NResults != 2 || len(request.QueryEmbeddings) != 1 {
				t.Fatalf("unexpected query request: %#v", request)
			}
			_ = json.NewEncoder(w).Encode(chromaQueryResponse{
				IDs:       [][]string{{"a", "b"}},
				Documents: [][]string{{"doc-a", "doc-b"}},
				Distances: [][]float32{{0.1, 0.2}},
				Metadatas: [][]map[string]any{{{"file_id": "file-a"}, {"file_id": "file-b"}}},
			})
		case r.Method == http.MethodPost && r.URL.Path == collectionIDPath+"/delete":
			var request struct {
				IDs []string `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode delete request: %v", err)
			}
			if strings.Join(request.IDs, ",") != "a,b" {
				t.Fatalf("unexpected delete ids: %#v", request.IDs)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewChroma(server.URL)
	result, err := client.Query(context.Background(), "memodrive", []float32{0.1, 0.2}, 2)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(result.IDs) != 2 || result.Documents[1] != "doc-b" || result.Distances[0] != 0.1 {
		t.Fatalf("unexpected query result: %#v", result)
	}
	if err := client.Delete(context.Background(), "memodrive", []string{"a", "b"}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if getCount.Load() != 2 {
		t.Fatalf("expected query lookup plus delete restore lookup, got %d lookups", getCount.Load())
	}
}
