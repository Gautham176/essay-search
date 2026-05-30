// Package embed wraps the Ollama HTTP API for generating embeddings.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"os"
)

// Default Ollama address and model. Override per-instance if needed.
const (
	DefaultBaseURL = "http://localhost:11434"
	DefaultModel   = "nomic-embed-text"
	// Dim is the embedding dimension for nomic-embed-text. Must match
	// the VECTOR(N) declaration in migrations/003_create_embeddings.sql.
	Dim = 768
)

// Client is a thin HTTP client for the Ollama embeddings endpoint.
type Client struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// NewClient returns a Client with sensible defaults. The HTTP client uses
// a 60s timeout — generous, since first calls can be slow if Ollama is
// loading the model into memory.
func NewClient() *Client {
    baseURL := DefaultBaseURL
    if env := os.Getenv("OLLAMA_URL"); env != "" {
        baseURL = env
    }
    return &Client{
        BaseURL: baseURL,
        Model:   DefaultModel,
        HTTP:    &http.Client{Timeout: 60 * time.Second},
    }
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed sends one piece of text to Ollama and returns its vector.
// Returns an error if the API call fails or the response has the wrong
// dimension.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Model: c.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.BaseURL+"/api/embeddings", bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read body for the error message — Ollama returns helpful JSON
		// errors with details when something's wrong.
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(buf))
	}

	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(out.Embedding) != Dim {
		return nil, fmt.Errorf("expected %d dims, got %d", Dim, len(out.Embedding))
	}
	return out.Embedding, nil
}