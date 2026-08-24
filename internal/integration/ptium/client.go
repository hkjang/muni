package ptium

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxResponseBytes = 64 << 20
	maxErrorBytes    = 32 << 10
)

// Client talks to a Ptium server. muni never touches Ptium's database: the
// presentation model belongs to Ptium and reaching past its API would break
// muni every time that model changes.
type Client struct {
	config Config
	http   *http.Client
}

func NewClient(config Config) *Client {
	config = config.Normalize()
	return &Client{
		config: config,
		http: &http.Client{
			Timeout: time.Duration(config.TimeoutSeconds) * time.Second,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (c *Client) Config() Config { return c.config }

// generateRequest is the body Ptium accepts to create and queue a deck.
type generateRequest struct {
	Title               string `json:"title"`
	Prompt              string `json:"prompt"`
	TemplateID          string `json:"templateId,omitempty"`
	Theme               string `json:"theme,omitempty"`
	Language            string `json:"language,omitempty"`
	Audience            string `json:"audience,omitempty"`
	Tone                string `json:"tone,omitempty"`
	RequestedSlideCount int    `json:"requestedSlideCount,omitempty"`
}

// Generate creates a presentation and asks Ptium to build it. Generation runs
// on Ptium's side, so this returns as soon as the deck is queued.
func (c *Client) Generate(ctx context.Context, brief Brief, options Options) (Presentation, error) {
	body := generateRequest{
		Title:               firstNonEmpty(options.Title, brief.Presentation.Title, brief.Source.Title, "Untitled presentation"),
		Prompt:              RenderPrompt(brief),
		TemplateID:          options.TemplateID,
		Theme:               firstNonEmpty(options.Theme, c.config.DefaultTheme),
		Language:            firstNonEmpty(options.Language, c.config.DefaultLang),
		Audience:            options.Audience,
		Tone:                options.Tone,
		RequestedSlideCount: options.SlideCount,
	}
	var presentation Presentation
	if err := c.do(ctx, http.MethodPost, "/api/v1/presentations/generate", body, &presentation); err != nil {
		return Presentation{}, err
	}
	return presentation, nil
}

// Get reads a presentation, which is how muni learns that generation finished.
func (c *Client) Get(ctx context.Context, id string) (Presentation, error) {
	var presentation Presentation
	if err := c.do(ctx, http.MethodGet, "/api/v1/presentations/"+id, nil, &presentation); err != nil {
		return Presentation{}, err
	}
	return presentation, nil
}

// Regenerate rebuilds an existing deck from a new prompt, which is how a
// document change reaches a presentation that already exists.
func (c *Client) Regenerate(ctx context.Context, id string, brief Brief, options Options) (Presentation, error) {
	body := generateRequest{
		Title:               firstNonEmpty(options.Title, brief.Presentation.Title, brief.Source.Title),
		Prompt:              RenderPrompt(brief),
		Theme:               firstNonEmpty(options.Theme, c.config.DefaultTheme),
		Language:            firstNonEmpty(options.Language, c.config.DefaultLang),
		Audience:            options.Audience,
		Tone:                options.Tone,
		RequestedSlideCount: options.SlideCount,
	}
	var presentation Presentation
	if err := c.do(ctx, http.MethodPost, "/api/v1/presentations/"+id+"/generate", body, &presentation); err != nil {
		return Presentation{}, err
	}
	return presentation, nil
}

// Delete removes a deck from Ptium.
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/presentations/"+id, nil, nil)
}

// Export streams the built file. The caller closes the reader.
func (c *Client) Export(ctx context.Context, id, format string) (io.ReadCloser, string, error) {
	if format != "pdf" {
		format = "pptx"
	}
	request, err := c.request(ctx, http.MethodGet, "/api/v1/presentations/"+id+"/export?format="+format, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("Ptium에 연결할 수 없습니다: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		return nil, "", readAPIError(response)
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return response.Body, contentType, nil
}

// Ping checks the credential and the address without creating anything.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/v1/me", nil, nil)
}

func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	if !c.config.Usable() {
		return nil, ErrNotConfigured
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	// Ptium accepts the key either way; the header form keeps it out of any
	// proxy log that records Authorization.
	request.Header.Set("X-API-Key", c.config.APIKey)
	return request, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	request, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("Ptium에 연결할 수 없습니다: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return readAPIError(response)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorBytes))
		return nil
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("Ptium 응답을 읽지 못했습니다: %w", err)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("Ptium 응답 형식을 이해하지 못했습니다: %s", truncate(string(payload), 300))
	}
	return nil
}

// readAPIError pulls the reason out of whichever envelope Ptium used.
func readAPIError(response *http.Response) error {
	payload, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBytes))
	apiError := &APIError{Status: response.StatusCode}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Title   string `json:"title"`
	}
	if json.Unmarshal(payload, &envelope) == nil {
		switch {
		case envelope.Error.Message != "":
			apiError.Code, apiError.Message = envelope.Error.Code, envelope.Error.Message
		case envelope.Message != "":
			apiError.Message = envelope.Message
		case envelope.Detail != "":
			apiError.Message = envelope.Detail
		case envelope.Title != "":
			apiError.Message = envelope.Title
		}
	}
	if apiError.Message == "" {
		apiError.Message = truncate(strings.TrimSpace(string(payload)), 300)
	}
	return apiError
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
