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
	"sort"
	"strings"
	"time"
)

const (
	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
	defaultOpenAIChatModel  = "gpt-4o"
	defaultOpenAIEmbedModel = "text-embedding-3-small"
)

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	BaseURL    string
	APIKey     string
	ChatModel  string
	EmbedModel string

	embedClient *http.Client
	chatClient  *http.Client
}

// NewOpenAI creates an OpenAI-compatible provider. If baseURL, chatModel, or embedModel
// are empty, sensible defaults are used.
func NewOpenAI(baseURL, apiKey, chatModel, embedModel string) *OpenAIProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOpenAIBaseURL
	}
	if strings.TrimSpace(chatModel) == "" {
		chatModel = defaultOpenAIChatModel
	}
	if strings.TrimSpace(embedModel) == "" {
		embedModel = defaultOpenAIEmbedModel
	}
	return &OpenAIProvider{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		APIKey:      strings.TrimSpace(apiKey),
		ChatModel:   chatModel,
		EmbedModel:  embedModel,
		embedClient: &http.Client{Timeout: 60 * time.Second},
		chatClient:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	started := time.Now()
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, fmt.Errorf("openai embed requires OPENAI_API_KEY")
	}
	if len(texts) == 0 {
		log.Printf("level=debug component=llm provider=openai event=embed_skipped reason=empty_input model=%q", p.EmbedModel)
		return [][]float32{}, nil
	}
	log.Printf("level=info component=llm provider=openai event=embed_begin model=%q inputs=%d base_url=%s", p.EmbedModel, len(texts), p.BaseURL)
	body := map[string]any{
		"model": p.EmbedModel,
		"input": texts,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/embeddings"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req)

	resp, err := p.embedClient.Do(req)
	if err != nil {
		log.Printf("level=error component=llm provider=openai event=embed_request_failed model=%q inputs=%d duration_ms=%d err=%q", p.EmbedModel, len(texts), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("openai embed request failed (baseURL: %s): %w", p.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := openAIHTTPError("embed", resp)
		log.Printf("level=error component=llm provider=openai event=embed_http_failed model=%q inputs=%d status=%d duration_ms=%d err=%q", p.EmbedModel, len(texts), resp.StatusCode, time.Since(started).Milliseconds(), err)
		return nil, err
	}

	var response struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("level=error component=llm provider=openai event=embed_decode_failed model=%q inputs=%d duration_ms=%d err=%q", p.EmbedModel, len(texts), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("decode openai embeddings response: %w", err)
	}
	if len(response.Data) != len(texts) {
		log.Printf("level=error component=llm provider=openai event=embed_count_mismatch model=%q inputs=%d returned=%d duration_ms=%d", p.EmbedModel, len(texts), len(response.Data), time.Since(started).Milliseconds())
		return nil, fmt.Errorf("openai embed returned %d embeddings for %d inputs", len(response.Data), len(texts))
	}
	sort.Slice(response.Data, func(i, j int) bool {
		return response.Data[i].Index < response.Data[j].Index
	})
	embeddings := make([][]float32, len(response.Data))
	for i, item := range response.Data {
		embeddings[i] = item.Embedding
	}
	dimensions := 0
	if len(embeddings) > 0 {
		dimensions = len(embeddings[0])
	}
	log.Printf("level=info component=llm provider=openai event=embed_complete model=%q inputs=%d dimensions=%d duration_ms=%d", p.EmbedModel, len(texts), dimensions, time.Since(started).Milliseconds())
	return embeddings, nil
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (<-chan string, error) {
	started := time.Now()
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, fmt.Errorf("openai chat requires OPENAI_API_KEY")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("openai chat requires at least one message")
	}
	log.Printf("level=info component=llm provider=openai event=chat_begin model=%q messages=%d base_url=%s", p.ChatModel, len(messages), p.BaseURL)
	body := map[string]any{
		"model":    p.ChatModel,
		"messages": messages,
		"stream":   true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/chat/completions"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req)

	resp, err := p.chatClient.Do(req)
	if err != nil {
		log.Printf("level=error component=llm provider=openai event=chat_request_failed model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("openai chat request failed (baseURL: %s): %w", p.BaseURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		err := openAIHTTPError("chat", resp)
		log.Printf("level=error component=llm provider=openai event=chat_http_failed model=%q messages=%d status=%d duration_ms=%d err=%q", p.ChatModel, len(messages), resp.StatusCode, time.Since(started).Milliseconds(), err)
		return nil, err
	}
	log.Printf("level=info component=llm provider=openai event=chat_stream_open model=%q messages=%d status=%d duration_ms=%d", p.ChatModel, len(messages), resp.StatusCode, time.Since(started).Milliseconds())

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
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				log.Printf("level=info component=llm provider=openai event=chat_stream_complete model=%q chunks=%d duration_ms=%d", p.ChatModel, chunkCount, time.Since(started).Milliseconds())
				return
			}
			var event struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Error *openAIError `json:"error,omitempty"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				log.Printf("level=error component=llm provider=openai event=chat_stream_parse_failed model=%q chunks=%d duration_ms=%d err=%q", p.ChatModel, chunkCount, time.Since(started).Milliseconds(), err)
				return
			}
			if event.Error != nil {
				log.Printf("level=error component=llm provider=openai event=chat_stream_error model=%q chunks=%d duration_ms=%d err=%q", p.ChatModel, chunkCount, time.Since(started).Milliseconds(), event.Error.Message)
				return
			}
			for _, choice := range event.Choices {
				if choice.Delta.Content == "" {
					continue
				}
				chunkCount++
				select {
				case chunks <- choice.Delta.Content:
				case <-ctx.Done():
					log.Printf("level=info component=llm provider=openai event=chat_stream_cancelled model=%q chunks=%d duration_ms=%d", p.ChatModel, chunkCount, time.Since(started).Milliseconds())
					return
				}
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			log.Printf("level=error component=llm provider=openai event=chat_stream_read_failed model=%q chunks=%d duration_ms=%d err=%q", p.ChatModel, chunkCount, time.Since(started).Milliseconds(), err)
			return
		}
		log.Printf("level=info component=llm provider=openai event=chat_stream_complete model=%q chunks=%d duration_ms=%d", p.ChatModel, chunkCount, time.Since(started).Milliseconds())
	}()
	return chunks, nil
}

func (p *OpenAIProvider) Complete(ctx context.Context, messages []Message) (string, error) {
	started := time.Now()
	if strings.TrimSpace(p.APIKey) == "" {
		return "", fmt.Errorf("openai complete requires OPENAI_API_KEY")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("openai complete requires at least one message")
	}
	log.Printf("level=info component=llm provider=openai event=complete_begin model=%q messages=%d base_url=%s", p.ChatModel, len(messages), p.BaseURL)
	body := map[string]any{
		"model":    p.ChatModel,
		"messages": messages,
		"stream":   false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/chat/completions"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	p.applyHeaders(req)

	resp, err := p.chatClient.Do(req)
	if err != nil {
		log.Printf("level=error component=llm provider=openai event=complete_request_failed model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), err)
		return "", fmt.Errorf("openai complete request failed (baseURL: %s): %w", p.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := openAIHTTPError("complete", resp)
		log.Printf("level=error component=llm provider=openai event=complete_http_failed model=%q messages=%d status=%d duration_ms=%d err=%q", p.ChatModel, len(messages), resp.StatusCode, time.Since(started).Milliseconds(), err)
		return "", err
	}
	var response struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Error *openAIError `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("level=error component=llm provider=openai event=complete_decode_failed model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), err)
		return "", fmt.Errorf("decode openai completion response: %w", err)
	}
	if response.Error != nil {
		log.Printf("level=error component=llm provider=openai event=complete_error model=%q messages=%d duration_ms=%d err=%q", p.ChatModel, len(messages), time.Since(started).Milliseconds(), response.Error.Message)
		return "", fmt.Errorf("openai complete failed: %s", response.Error.Message)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("openai complete returned no choices")
	}
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	log.Printf("level=info component=llm provider=openai event=complete_complete model=%q messages=%d chars=%d duration_ms=%d", p.ChatModel, len(messages), len([]rune(content)), time.Since(started).Milliseconds())
	return content, nil
}

func (p *OpenAIProvider) endpoint(path string) string {
	return p.BaseURL + "/" + strings.TrimLeft(path, "/")
}

func (p *OpenAIProvider) applyHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func openAIHTTPError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	message := strings.TrimSpace(string(body))
	var parsed struct {
		Error openAIError `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
		if parsed.Error.Type != "" {
			message = fmt.Sprintf("%s (%s)", parsed.Error.Message, parsed.Error.Type)
		}
	}
	if message == "" {
		message = resp.Status
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("openai %s authentication failed (status: %d): %s", operation, resp.StatusCode, message)
	case http.StatusTooManyRequests:
		return fmt.Errorf("openai %s rate limited (status: %d): %s", operation, resp.StatusCode, message)
	default:
		return fmt.Errorf("openai %s failed (status: %d): %s", operation, resp.StatusCode, message)
	}
}
