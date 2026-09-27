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
