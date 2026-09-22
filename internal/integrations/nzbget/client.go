package nzbget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
)

type Client struct {
	http *http.Client
}

type rpcRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int    `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
	ID     int             `json:"id"`
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) Call(
	ctx context.Context,
	baseURL string,
	username string,
	password string,
	method string,
	result any,
	params ...any,
) error {
	normalized, err := integrations.NormalizeBaseURL(baseURL)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(rpcRequest{
		Method: method,
		Params: params,
		ID:     1,
	})
	if err != nil {
		return fmt.Errorf("encode NZBGet request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		normalized+"/jsonrpc",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create NZBGet request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Overmynd/dev")
	req.SetBasicAuth(username, password)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("NZBGet request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("read NZBGet response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"NZBGet returned HTTP %d: %s",
			resp.StatusCode,
			string(body),
		)
	}

	var response rpcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decode NZBGet response: %w", err)
	}

	if len(response.Error) > 0 &&
		string(response.Error) != "null" {
		return fmt.Errorf(
			"NZBGet RPC error: %s",
			string(response.Error),
		)
	}

	if result == nil {
		return nil
	}

	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf(
			"decode NZBGet %s result: %w",
			method,
			err,
		)
	}

	return nil
}
