package homarr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// trpcMutation calls a tRPC mutation: POST /api/trpc/<procedure> with {"json": input}
func (c *Client) trpcMutation(ctx context.Context, procedure string, input any, result any) error {
	body := map[string]any{"json": input}
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal tRPC input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/trpc/"+procedure, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("ApiKey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tRPC %s: %w", procedure, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return &NotFoundError{Procedure: procedure}
		}
		return fmt.Errorf("tRPC %s returned %d: %s", procedure, resp.StatusCode, string(respBody))
	}

	if result != nil {
		// tRPC wraps response in {"result": {"data": {"json": ...}}}
		var wrapper struct {
			Result struct {
				Data struct {
					JSON json.RawMessage `json:"json"`
				} `json:"data"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			return fmt.Errorf("decode tRPC response: %w", err)
		}
		if err := json.Unmarshal(wrapper.Result.Data.JSON, result); err != nil {
			return fmt.Errorf("unmarshal tRPC result: %w", err)
		}
	}
	return nil
}

// trpcQuery calls a tRPC query: GET /api/trpc/<procedure>?input={"json": input}
func (c *Client) trpcQuery(ctx context.Context, procedure string, input any, result any) error {
	reqURL := c.baseURL + "/api/trpc/" + procedure
	if input != nil {
		wrapper := map[string]any{"json": input}
		b, err := json.Marshal(wrapper)
		if err != nil {
			return fmt.Errorf("marshal tRPC query input: %w", err)
		}
		reqURL += "?input=" + url.QueryEscape(string(b))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("ApiKey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tRPC query %s: %w", procedure, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return &NotFoundError{Procedure: procedure}
		}
		return fmt.Errorf("tRPC query %s returned %d: %s", procedure, resp.StatusCode, string(respBody))
	}

	if result != nil {
		var wrapper struct {
			Result struct {
				Data struct {
					JSON json.RawMessage `json:"json"`
				} `json:"data"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			return fmt.Errorf("decode tRPC response: %w", err)
		}
		if err := json.Unmarshal(wrapper.Result.Data.JSON, result); err != nil {
			return fmt.Errorf("unmarshal tRPC result: %w", err)
		}
	}
	return nil
}

type NotFoundError struct {
	Procedure string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.Procedure)
}

func IsNotFound(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}
