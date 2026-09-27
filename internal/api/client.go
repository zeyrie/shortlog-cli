package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func (c *Client) Origin() string { return c.baseURL }

func NewClient(address string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("API URL must be an http(s) origin without a path, query, or credentials")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(u.String(), "/"), http: httpClient}, nil
}

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("API request failed (%d, %s)", e.Status, e.Code)
}

func (c *Client) post(ctx context.Context, path string, input, output any, expected int) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "shortlog-cli")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 64*1024)
	if resp.StatusCode != expected {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(limited).Decode(&envelope)
		return &Error{Status: resp.StatusCode, Code: envelope.Error.Code}
	}
	if err := json.NewDecoder(limited).Decode(output); err != nil {
		return fmt.Errorf("invalid API response: %w", err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path, token string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "shortlog-cli")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&envelope)
		return &Error{Status: resp.StatusCode, Code: envelope.Error.Code}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(output); err != nil {
		return fmt.Errorf("invalid API response: %w", err)
	}
	return nil
}

type Account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (c *Client) Me(ctx context.Context, token string) (Account, error) {
	var result Account
	err := c.get(ctx, "/v1/me", token, &result)
	if err == nil && result.ID == "" {
		err = errors.New("invalid API response: missing account ID")
	}
	return result, err
}

type Note struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type NotesPage struct {
	Items      []Note  `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func (c *Client) Inbox(ctx context.Context, token string) (NotesPage, error) {
	var result NotesPage
	err := c.get(ctx, "/v1/notes", token, &result)
	return result, err
}

func (c *Client) InboxPage(ctx context.Context, token, cursor string) (NotesPage, error) {
	if cursor == "" {
		return NotesPage{}, errors.New("missing Inbox cursor")
	}
	var result NotesPage
	err := c.get(ctx, "/v1/notes?cursor="+url.QueryEscape(cursor), token, &result)
	return result, err
}

func (c *Client) CreateInboxNote(ctx context.Context, token, content string) (Note, error) {
	var result Note
	err := c.postAuthorized(ctx, "/v1/notes", token, map[string]string{"content": content}, &result, http.StatusCreated)
	if err == nil && result.ID == "" {
		err = errors.New("invalid API response: missing note ID")
	}
	return result, err
}

func (c *Client) postAuthorized(ctx context.Context, path, token string, input, output any, expected int) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "shortlog-cli")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&envelope)
		return &Error{Status: resp.StatusCode, Code: envelope.Error.Code}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(output); err != nil {
		return fmt.Errorf("invalid API response: %w", err)
	}
	return nil
}

func (c *Client) StartEmail(ctx context.Context, email string) (string, error) {
	var result struct {
		ChallengeID string `json:"challenge_id"`
	}
	err := c.post(ctx, "/v1/auth/email/start", map[string]string{"email": email}, &result, http.StatusAccepted)
	if err == nil && result.ChallengeID == "" {
		err = errors.New("invalid API response: missing challenge ID")
	}
	return result.ChallengeID, err
}

type VerifyResult struct {
	Status         string `json:"status"`
	Token          string `json:"token"`
	RecoveryTicket string `json:"recovery_ticket"`
}

func (c *Client) VerifyEmail(ctx context.Context, challengeID, code, username, timeZone string) (VerifyResult, error) {
	input := map[string]string{"challenge_id": challengeID, "code": code}
	if username != "" || timeZone != "" {
		input["username"] = username
		input["time_zone"] = timeZone
	}
	var result VerifyResult
	err := c.post(ctx, "/v1/auth/email/verify", input, &result, http.StatusOK)
	if err == nil && !((result.Status == "signed_in" && result.Token != "") || (result.Status == "restore_required" && result.RecoveryTicket != "")) {
		err = errors.New("invalid API response: unexpected verification result")
	}
	return result, err
}

func (c *Client) RestoreEmail(ctx context.Context, ticket string) (string, error) {
	var result struct {
		Token string `json:"token"`
	}
	err := c.post(ctx, "/v1/auth/email/restore", map[string]string{"recovery_ticket": ticket}, &result, http.StatusOK)
	if err == nil && result.Token == "" {
		err = errors.New("invalid API response: missing token")
	}
	return result.Token, err
}
