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

// Health reports whether the server answers its health check, which needs
// no session: a quick way to tell a reachable Shortlog server from a typo.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return &Error{Status: resp.StatusCode, Code: "unhealthy"}
	}
	return nil
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	TimeZone  string    `json:"time_zone"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) Me(ctx context.Context, token string) (Account, error) {
	var result Account
	err := c.get(ctx, "/v1/me", token, &result)
	if err == nil && result.ID == "" {
		err = errors.New("invalid API response: missing account ID")
	}
	return result, err
}

func (c *Client) UpdateProfile(ctx context.Context, token, username, timeZone string) (Account, error) {
	var result Account
	err := c.writeNote(ctx, http.MethodPatch, "/v1/me", token, map[string]string{"username": username, "time_zone": timeZone}, &result, http.StatusOK)
	if err == nil && result.ID == "" {
		err = errors.New("invalid API response: missing account ID")
	}
	return result, err
}

func (c *Client) RequestAccountDeletion(ctx context.Context, token string) (time.Time, error) {
	var result struct {
		DeletionScheduledFor time.Time `json:"deletion_scheduled_for"`
	}
	err := c.writeNote(ctx, http.MethodDelete, "/v1/me", token, nil, &result, http.StatusAccepted)
	if err == nil && result.DeletionScheduledFor.IsZero() {
		err = errors.New("invalid API response: missing deletion deadline")
	}
	return result.DeletionScheduledFor, err
}

type Session struct {
	ID              string    `json:"id"`
	UserAgent       string    `json:"user_agent"`
	DeviceLabel     string    `json:"device_label"`
	CreatedAt       time.Time `json:"created_at"`
	LastUsedAt      time.Time `json:"last_used_at"`
	AuthenticatedAt time.Time `json:"authenticated_at"`
	Current         bool      `json:"current"`
}

func (c *Client) Sessions(ctx context.Context, token string) ([]Session, error) {
	var sessions []Session
	err := c.get(ctx, "/v1/sessions", token, &sessions)
	return sessions, err
}

func (c *Client) Logout(ctx context.Context, token string) error {
	return c.writeNote(ctx, http.MethodPost, "/v1/auth/logout", token, nil, nil, http.StatusNoContent)
}

func (c *Client) RevokeSession(ctx context.Context, token, id string) error {
	return c.writeNote(ctx, http.MethodDelete, "/v1/sessions/"+url.PathEscape(id), token, nil, nil, http.StatusNoContent)
}

func (c *Client) RevokeAllSessions(ctx context.Context, token string) error {
	return c.writeNote(ctx, http.MethodPost, "/v1/sessions/revoke-all", token, nil, nil, http.StatusNoContent)
}

type Note struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	ProjectID *string   `json:"project_id"`
	CreatedAt time.Time `json:"created_at"`
}

type NotesPage struct {
	Items      []Note  `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type Project struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ArchivedAt *time.Time `json:"archived_at"`
}

func (c *Client) Projects(ctx context.Context, token string) ([]Project, error) {
	var projects []Project
	err := c.get(ctx, "/v1/projects", token, &projects)
	return projects, err
}

func (c *Client) ArchivedProjects(ctx context.Context, token string) ([]Project, error) {
	var projects []Project
	err := c.get(ctx, "/v1/projects?status=archived", token, &projects)
	return projects, err
}

func (c *Client) ArchiveProject(ctx context.Context, token, id string) error {
	return c.writeNote(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(id)+"/archive", token, nil, nil, http.StatusNoContent)
}

func (c *Client) UnarchiveProject(ctx context.Context, token, id string) error {
	return c.writeNote(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(id)+"/unarchive", token, nil, nil, http.StatusNoContent)
}

func (c *Client) CreateProject(ctx context.Context, token, name string) (Project, error) {
	var project Project
	err := c.postAuthorized(ctx, "/v1/projects", token, map[string]string{"name": name}, &project, http.StatusCreated)
	if err == nil && project.ID == "" {
		err = errors.New("invalid API response: missing project ID")
	}
	return project, err
}

func (c *Client) ProjectNotes(ctx context.Context, token, projectID string) (NotesPage, error) {
	var page NotesPage
	err := c.get(ctx, "/v1/projects/"+url.PathEscape(projectID)+"/notes", token, &page)
	return page, err
}

func (c *Client) ProjectNotesPage(ctx context.Context, token, projectID, cursor string) (NotesPage, error) {
	if cursor == "" {
		return NotesPage{}, errors.New("missing project notes cursor")
	}
	var page NotesPage
	err := c.get(ctx, "/v1/projects/"+url.PathEscape(projectID)+"/notes?cursor="+url.QueryEscape(cursor), token, &page)
	return page, err
}

func (c *Client) CreateProjectNote(ctx context.Context, token, projectID, content string) (Note, error) {
	var result Note
	err := c.postAuthorized(ctx, "/v1/notes", token, map[string]string{"content": content, "project_id": projectID}, &result, http.StatusCreated)
	if err == nil && result.ID == "" {
		err = errors.New("invalid API response: missing note ID")
	}
	return result, err
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

func (c *Client) UpdateNote(ctx context.Context, token, id, content string) (Note, error) {
	var result Note
	err := c.writeNote(ctx, http.MethodPatch, "/v1/notes/"+url.PathEscape(id), token, map[string]string{"content": content}, &result, http.StatusOK)
	if err == nil && result.ID != id {
		err = errors.New("invalid API response: mismatched note ID")
	}
	return result, err
}

func (c *Client) MoveNote(ctx context.Context, token, id, projectID string) (Note, error) {
	var destination any
	if projectID != "" {
		destination = projectID
	}
	var result Note
	err := c.writeNote(ctx, http.MethodPatch, "/v1/notes/"+url.PathEscape(id), token, map[string]any{"project_id": destination}, &result, http.StatusOK)
	if err == nil && result.ID != id {
		err = errors.New("invalid API response: mismatched note ID")
	}
	return result, err
}

func (c *Client) DeleteNote(ctx context.Context, token, id string) error {
	return c.writeNote(ctx, http.MethodDelete, "/v1/notes/"+url.PathEscape(id), token, nil, nil, http.StatusNoContent)
}

func (c *Client) writeNote(ctx context.Context, method, path, token string, input, output any, expected int) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "shortlog-cli")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
	if output != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(output); err != nil {
			return fmt.Errorf("invalid API response: %w", err)
		}
	}
	return nil
}

func (c *Client) postAuthorized(ctx context.Context, path, token string, input, output any, expected int) error {
	return c.writeNote(ctx, http.MethodPost, path, token, input, output, expected)
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

type TelegramStart struct {
	AttemptID        string `json:"attempt_id"`
	PollSecret       string `json:"poll_secret"`
	AuthorizationURL string `json:"authorization_url"`
}

func (c *Client) StartTelegram(ctx context.Context) (TelegramStart, error) {
	var result TelegramStart
	err := c.writeNote(ctx, http.MethodPost, "/v1/auth/telegram/start", "", nil, &result, http.StatusCreated)
	if err == nil && (result.AttemptID == "" || result.PollSecret == "" || result.AuthorizationURL == "") {
		err = errors.New("invalid API response: incomplete Telegram attempt")
	}
	return result, err
}

func (c *Client) PollTelegram(ctx context.Context, attemptID, pollSecret, username, timeZone string) (VerifyResult, error) {
	input := map[string]string{"attempt_id": attemptID, "poll_secret": pollSecret}
	if username != "" || timeZone != "" {
		input["username"], input["time_zone"] = username, timeZone
	}
	var result VerifyResult
	// Pending polls return 202; completed polls return 200. Both carry JSON.
	err := c.writeTelegramPoll(ctx, input, &result)
	if err == nil && !((result.Status == "pending") || (result.Status == "signed_in" && result.Token != "") || (result.Status == "restore_required" && result.RecoveryTicket != "")) {
		err = errors.New("invalid API response: unexpected Telegram poll result")
	}
	return result, err
}

func (c *Client) writeTelegramPoll(ctx context.Context, input any, output *VerifyResult) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/auth/telegram/poll", bytes.NewReader(data))
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&envelope)
		return &Error{Status: resp.StatusCode, Code: envelope.Error.Code}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(output); err != nil {
		return fmt.Errorf("invalid API response: %w", err)
	}
	if (resp.StatusCode == http.StatusAccepted) != (output.Status == "pending") {
		return errors.New("invalid API response: Telegram poll status mismatch")
	}
	return nil
}

func (c *Client) RestoreTelegram(ctx context.Context, ticket string) (string, error) {
	var result struct {
		Token string `json:"token"`
	}
	err := c.post(ctx, "/v1/auth/telegram/restore", map[string]string{"recovery_ticket": ticket}, &result, http.StatusOK)
	if err == nil && result.Token == "" {
		err = errors.New("invalid API response: missing token")
	}
	return result.Token, err
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
