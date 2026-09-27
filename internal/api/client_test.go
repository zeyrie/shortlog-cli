package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmailFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/v1/auth/email/start":
			if body["email"] != "a@example.com" {
				t.Errorf("start body: %v", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"challenge_id":"challenge"}`))
		case "/v1/auth/email/verify":
			if body["challenge_id"] != "challenge" || body["code"] != "12345678" {
				t.Errorf("verify body: %v", body)
			}
			if body["username"] == "" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":{"code":"profile_required","message":"Profile required."}}`))
				return
			}
			if body["time_zone"] != "Europe/London" {
				t.Errorf("profile body: %v", body)
			}
			_, _ = w.Write([]byte(`{"status":"signed_in","token":"secret-token"}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.StartEmail(context.Background(), "a@example.com")
	if err != nil || id != "challenge" {
		t.Fatalf("start: %q, %v", id, err)
	}
	_, err = client.VerifyEmail(context.Background(), id, "12345678", "", "")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "profile_required" {
		t.Fatalf("expected profile_required, got %v", err)
	}
	result, err := client.VerifyEmail(context.Background(), id, "12345678", "Ari", "Europe/London")
	if err != nil || result.Token != "secret-token" {
		t.Fatalf("verify: %+v, %v", result, err)
	}
}

func TestRestore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/email/restore" {
			t.Errorf("path: %s", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["recovery_ticket"] != "ticket" {
			t.Errorf("restore body: %v", body)
		}
		_, _ = w.Write([]byte(`{"token":"new-token"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.RestoreEmail(context.Background(), "ticket")
	if err != nil || token != "new-token" {
		t.Fatalf("restore: %q, %v", token, err)
	}
}

func TestRejectsInvalidOrigin(t *testing.T) {
	for _, value := range []string{"not-a-url", "https://user:pass@example.com", "https://example.com/v1", "https://example.com?token=secret"} {
		if _, err := NewClient(value, nil); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestAuthenticatedInbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer saved-token" {
			t.Errorf("unexpected auth request: %s %q", r.Method, r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/me":
			_, _ = w.Write([]byte(`{"id":"account","username":"Ari"}`))
		case "/v1/notes":
			_, _ = w.Write([]byte(`{"items":[{"id":"one","content":"Hello","created_at":"2026-09-28T12:00:00Z"}],"next_cursor":"later"}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	account, err := client.Me(context.Background(), "saved-token")
	if err != nil || account.Username != "Ari" {
		t.Fatalf("me: %+v, %v", account, err)
	}
	page, err := client.Inbox(context.Background(), "saved-token")
	if err != nil || len(page.Items) != 1 || page.Items[0].Content != "Hello" || page.NextCursor == nil {
		t.Fatalf("inbox: %+v, %v", page, err)
	}
}

func TestUnauthorizedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"Authentication required."}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Me(context.Background(), "expired")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Code != "unauthorized" {
		t.Fatalf("expected unauthorized: %v", err)
	}
}

func TestCreateInboxNote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/notes" || r.Header.Get("Authorization") != "Bearer session" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if len(input) != 1 || input["content"] != "First\nSecond" {
			t.Errorf("content: %#v", input)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-note","content":"First\nSecond","created_at":"2026-09-28T12:00:00Z"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	note, err := client.CreateInboxNote(context.Background(), "session", "First\nSecond")
	if err != nil || note.ID != "new-note" || note.Content != "First\nSecond" {
		t.Fatalf("create: %+v, %v", note, err)
	}
}
