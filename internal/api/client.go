package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	baseURL    = "https://api.earthmc.net/v3/aurora"
	mapBaseURL = "https://map.earthmc.net/tiles/players.json"
	batchSize  = 100
)

// Client wraps HTTP calls to the EarthMC API and map.
type Client struct {
	http *http.Client
}

// NewClient creates a new API client with sensible timeouts.
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ---- GET helpers ----

func (c *Client) doGet(ctx context.Context, url string, out interface{}) error {
	return c.doWithRetry(ctx, "GET", url, nil, out)
}

func (c *Client) doPost(ctx context.Context, url string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	return c.doWithRetry(ctx, "POST", url, data, out)
}

func (c *Client) doWithRetry(ctx context.Context, method, url string, body []byte, out interface{}) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Debug("retrying request", "attempt", attempt+1, "backoff", backoff, "url", url)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("do request: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
			continue
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("client error %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
		}

		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("unmarshal response from %s: %w", url, err)
		}
		return nil
	}
	return fmt.Errorf("all retries exhausted for %s %s: %w", method, url, lastErr)
}

// ---- Public API methods ----

// GetServer fetches the server status.
func (c *Client) GetServer(ctx context.Context) (*ServerResponse, error) {
	var resp ServerResponse
	if err := c.doGet(ctx, baseURL+"/", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOnline fetches currently online players.
func (c *Client) GetOnline(ctx context.Context) (*OnlineResponse, error) {
	var resp OnlineResponse
	if err := c.doGet(ctx, baseURL+"/online", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetMapPlayers fetches the map's live player positions.
func (c *Client) GetMapPlayers(ctx context.Context) (*MapPlayersResponse, error) {
	var resp MapPlayersResponse
	if err := c.doGet(ctx, mapBaseURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTownsList fetches the list of all towns (name + uuid only).
func (c *Client) GetTownsList(ctx context.Context) ([]ListEntry, error) {
	var resp []ListEntry
	if err := c.doGet(ctx, baseURL+"/towns", &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetNationsList fetches the list of all nations (name + uuid only).
func (c *Client) GetNationsList(ctx context.Context) ([]ListEntry, error) {
	var resp []ListEntry
	if err := c.doGet(ctx, baseURL+"/nations", &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPlayersList fetches the list of all players (name + uuid only).
func (c *Client) GetPlayersList(ctx context.Context) ([]ListEntry, error) {
	var resp []ListEntry
	if err := c.doGet(ctx, baseURL+"/players", &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// PostTowns fetches detailed town data for the given UUIDs (batched).
func (c *Client) PostTowns(ctx context.Context, uuids []string) ([]json.RawMessage, error) {
	return c.batchPost(ctx, baseURL+"/towns", uuids)
}

// PostNations fetches detailed nation data for the given UUIDs (batched).
func (c *Client) PostNations(ctx context.Context, uuids []string) ([]json.RawMessage, error) {
	return c.batchPost(ctx, baseURL+"/nations", uuids)
}

// PostPlayers fetches detailed player data for the given UUIDs (batched).
func (c *Client) PostPlayers(ctx context.Context, uuids []string) ([]json.RawMessage, error) {
	return c.batchPost(ctx, baseURL+"/players", uuids)
}

// batchPost sends POST requests in batches and collects all results.
func (c *Client) batchPost(ctx context.Context, url string, uuids []string) ([]json.RawMessage, error) {
	var allResults []json.RawMessage

	for i := 0; i < len(uuids); i += batchSize {
		end := i + batchSize
		if end > len(uuids) {
			end = len(uuids)
		}
		batch := uuids[i:end]

		body := PostQuery{Query: batch}
		var results []json.RawMessage
		if err := c.doPost(ctx, url, body, &results); err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}
