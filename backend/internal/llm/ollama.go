package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	defaultOllamaBaseURL    = "http://ollama:11434"
	defaultOllamaChatModel  = "qwen2.5:1.5b"
	defaultOllamaEmbedModel = "nomic-embed-text"
)

// OllamaProvider implements Provider for a local Ollama server.
type OllamaProvider struct {
	BaseURL    string
	ChatModel  string
	EmbedModel string

	embedClient *http.Client
	chatClient  *http.Client
}

// NewOllama creates an Ollama provider. If baseURL, chatModel, or embedModel
// are empty, sensible defaults are used.
func NewOllama(baseURL, chatModel, embedModel string) *OllamaProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOllamaBaseURL
	}
	if strings.TrimSpace(chatModel) == "" {
		chatModel = defaultOllamaChatModel
	}
	if strings.TrimSpace(embedModel) == "" {
		embedModel = defaultOllamaEmbedModel
	}
	return &OllamaProvider{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		ChatModel:   chatModel,
		EmbedModel:  embedModel,
		embedClient: &http.Client{Timeout: 60 * time.Second},
		chatClient:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	started := time.Now()
	if len(texts) == 0 {
		log.Printf("level=debug component=llm provider=ollama event=embed_skipped reason=empty_input model=%q", p.EmbedModel)
		return [][]float32{}, nil
	}
	log.Printf("level=info component=llm provider=ollama event=embed_begin model=%q inputs=%d base_url=%s", p.EmbedModel, len(texts), p.BaseURL)
	body := map[string]any{
		"model": p.EmbedModel,
		"input": texts,
	}
	var response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := p.doJSON(ctx, p.embedClient, "/api/embed", body, &response); err != nil {
		log.Printf("level=error component=llm provider=ollama event=embed_failed model=%q inputs=%d duration_ms=%d err=%q", p.EmbedModel, len(texts), time.Since(started).Milliseconds(), err)
		return nil, err
	}
	if len(response.Embeddings) != len(texts) {
		log.Printf("level=error component=llm provider=ollama event=embed_count_mismatch model=%q inputs=%d returned=%d duration_ms=%d", p.EmbedModel, len(texts), len(response.Embeddings), time.Since(started).Milliseconds())
		return nil, fmt.Errorf("ollama embed returned %d embeddings for %d inputs", len(response.Embeddings), len(texts))
	}
	dimensions := 0
	if len(response.Embeddings) > 0 {
		dimensions = len(response.Embeddings[0])
	}
	log.Printf("level=info component=llm provider=ollama event=embed_complete model=%q inputs=%d dimensions=%d duration_ms=%d", p.EmbedModel, len(texts), dimensions, time.Since(started).Milliseconds())
	return response.Embeddings, nil
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (<-chan string, error) {
	started := time.Now()
	if len(messages) == 0 {
		return nil, fmt.Errorf("ollama chat requires at least one message")
	}
	log.Printf("level=info component=llm provider=ollama event=chat_begin model=%q messages=%d base_url=%s", p.ChatModel, len(messages), p.BaseURL)
	body := map[string]any{
		"model":    p.ChatModel,
		"messages": messages,
		"stream":   true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/api/chat"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.chatClient.Do(req)
	if err != nil {
		log.Printf("level=error component=llm provider=ollama event=chat_request_failed model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("ollama chat request failed (baseURL: %s): %w", p.BaseURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		err := ollamaHTTPError("chat", p.BaseURL, resp)
		log.Printf("level=error component=llm provider=ollama event=chat_http_failed model=%q messages=%d status=%d duration_ms=%d err=%q", p.ChatModel, len(messages), resp.StatusCode, time.Since(started).Milliseconds(), err)
		return nil, err
	}
	log.Printf("level=info component=llm provider=ollama event=chat_stream_open model=%q messages=%d status=%d duration_ms=%d", p.ChatModel, len(messages), resp.StatusCode, time.Since(started).Milliseconds())

	chunks := make(chan string, 8)
	go func() {
		defer close(chunks)
		defer resp.Body.Close()

		chunkCount := 0
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var chunk struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done  bool   `json:"done"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				log.Printf("level=error component=llm provider=ollama event=chat_stream_parse_failed model=%q chunks=%d duration_ms=%d err=%q", p.ChatModel, chunkCount, time.Since(started).Milliseconds(), err)
				return
			}
			if chunk.Error != "" {
				log.Printf("level=error component=llm provider=ollama event=chat_stream_error model=%q chunks=%d duration_ms=%d err=%q", p.ChatModel, chunkCount, time.Since(started).Milliseconds(), chunk.Error)
				return
			}
			if chunk.Message.Content != "" {
				chunkCount++
				select {
				case chunks <- chunk.Message.Content:
				case <-ctx.Done():
					log.Printf("level=info component=llm provider=ollama event=chat_stream_cancelled model=%q chunks=%d duration_ms=%d", p.ChatModel, chunkCount, time.Since(started).Milliseconds())
					return
				}
			}
			if chunk.Done {
				log.Printf("level=info component=llm provider=ollama event=chat_stream_complete model=%q chunks=%d duration_ms=%d", p.ChatModel, chunkCount, time.Since(started).Milliseconds())
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			log.Printf("level=error component=llm provider=ollama event=chat_stream_read_failed model=%q chunks=%d duration_ms=%d err=%q", p.ChatModel, chunkCount, time.Since(started).Milliseconds(), err)
			return
		}
		log.Printf("level=info component=llm provider=ollama event=chat_stream_complete model=%q chunks=%d duration_ms=%d", p.ChatModel, chunkCount, time.Since(started).Milliseconds())
	}()
	return chunks, nil
}

func (p *OllamaProvider) Complete(ctx context.Context, messages []Message) (string, error) {
	started := time.Now()
	if len(messages) == 0 {
		return "", fmt.Errorf("ollama complete requires at least one message")
	}
	log.Printf("level=info component=llm provider=ollama event=complete_begin model=%q messages=%d base_url=%s", p.ChatModel, len(messages), p.BaseURL)
	body := map[string]any{
		"model":    p.ChatModel,
		"messages": messages,
		"stream":   false,
	}
	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := p.doJSON(ctx, p.chatClient, "/api/chat", body, &response); err != nil {
		log.Printf("level=error component=llm provider=ollama event=complete_failed model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), err)
		return "", err
	}
	if response.Error != "" {
		log.Printf("level=error component=llm provider=ollama event=complete_error model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), response.Error)
		return "", fmt.Errorf("ollama complete failed: %s", response.Error)
	}
	content := strings.TrimSpace(response.Message.Content)
	log.Printf("level=info component=llm provider=ollama event=complete_complete model=%q messages=%d chars=%d duration_ms=%d", p.ChatModel, len(messages), len([]rune(content)), time.Since(started).Milliseconds())
	return content, nil
}

func (p *OllamaProvider) doJSON(ctx context.Context, client *http.Client, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(path), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama request failed (baseURL: %s): %w", p.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ollamaHTTPError(path, p.BaseURL, resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode ollama response: %w", err)
	}
	return nil
}

func (p *OllamaProvider) endpoint(path string) string {
	return p.BaseURL + "/" + strings.TrimLeft(path, "/")
}

func ollamaHTTPError(operation, baseURL string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("ollama %s failed (baseURL: %s, status: %d): %s", operation, baseURL, resp.StatusCode, message)
}
