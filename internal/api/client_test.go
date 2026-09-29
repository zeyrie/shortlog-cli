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

func TestInboxCursorIsEncodedUnchanged(t *testing.T) {
	cursor := "opaque+/= &?"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notes" || r.URL.Query().Get("cursor") != cursor || len(r.URL.Query()) != 1 || r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("unexpected cursor request: %s %q", r.URL.String(), r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"items":[],"next_cursor":null}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.InboxPage(context.Background(), "session", cursor)
	if err != nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatalf("page: %+v, %v", page, err)
	}
	if _, err := client.InboxPage(context.Background(), "session", ""); err == nil {
		t.Fatal("empty cursor silently refreshed first page")
	}
}

func TestEditAndDeleteNote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notes/note-id" || r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("unexpected request: %s, %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		switch r.Method {
		case http.MethodPatch:
			var fields map[string]string
			if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
				t.Fatal(err)
			}
			if len(fields) != 1 || fields["content"] != "changed\nline" {
				t.Errorf("patch fields: %v", fields)
			}
			_, _ = w.Write([]byte(`{"id":"note-id","content":"changed\nline","created_at":"2026-09-28T12:00:00Z"}`))
		case http.MethodDelete:
			if r.ContentLength > 0 {
				t.Error("delete sent a body")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	note, err := client.UpdateNote(context.Background(), "session", "note-id", "changed\nline")
	if err != nil || note.ID != "note-id" || note.Content != "changed\nline" {
		t.Fatalf("patch: %+v, %v", note, err)
	}
	if err := client.DeleteNote(context.Background(), "session", "note-id"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestDeleteNotFoundIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"Not found."}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	err = client.DeleteNote(context.Background(), "session", "missing")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound || apiErr.Code != "not_found" {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestProjectEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("missing auth header for %s", r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects":
			_, _ = w.Write([]byte(`[{"id":"p1","name":"Work"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if len(input) != 1 || input["name"] != "Research" {
				t.Errorf("create body: %v", input)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"p2","name":"Research"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/p1/notes":
			if cursor := r.URL.Query().Get("cursor"); cursor != "" {
				if cursor != "opaque &?" || len(r.URL.Query()) != 1 {
					t.Errorf("cursor query: %q", r.URL.RawQuery)
				}
				_, _ = w.Write([]byte(`{"items":[],"next_cursor":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"n1","content":"hello"}],"next_cursor":"opaque &?"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes":
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["project_id"] != "p1" || input["content"] != "project note" {
				t.Errorf("note body: %v", input)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"n2","content":"project note","project_id":"p1"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	projects, err := client.Projects(context.Background(), "session")
	if err != nil || len(projects) != 1 || projects[0].ID != "p1" {
		t.Fatalf("projects: %+v, %v", projects, err)
	}
	created, err := client.CreateProject(context.Background(), "session", "Research")
	if err != nil || created.ID != "p2" || created.Name != "Research" {
		t.Fatalf("create project: %+v, %v", created, err)
	}
	page, err := client.ProjectNotes(context.Background(), "session", "p1")
	if err != nil || len(page.Items) != 1 || derefCursor(page.NextCursor) != "opaque &?" {
		t.Fatalf("project notes: %+v, %v", page, err)
	}
	older, err := client.ProjectNotesPage(context.Background(), "session", "p1", "opaque &?")
	if err != nil || len(older.Items) != 0 {
		t.Fatalf("project notes page: %+v, %v", older, err)
	}
	if _, err := client.ProjectNotesPage(context.Background(), "session", "p1", ""); err == nil {
		t.Fatal("empty cursor silently refreshed first page")
	}
	note, err := client.CreateProjectNote(context.Background(), "session", "p1", "project note")
	if err != nil || note.ID != "n2" {
		t.Fatalf("project note create: %+v, %v", note, err)
	}
}

func derefCursor(cursor *string) string {
	if cursor == nil {
		return ""
	}
	return *cursor
}

func TestMoveNoteProjectAndInbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/notes/note-id" || r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("unexpected move request: %s %s %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var fields map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			t.Error(err)
			return
		}
		if len(fields) != 1 {
			t.Errorf("move must patch only project_id: %v", fields)
		}
		switch string(fields["project_id"]) {
		case `"p1"`:
			_, _ = w.Write([]byte(`{"id":"note-id","content":"untouched","project_id":"p1"}`))
		case `null`:
			_, _ = w.Write([]byte(`{"id":"note-id","content":"untouched","project_id":null}`))
		default:
			t.Errorf("unexpected project_id JSON: %s", fields["project_id"])
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	inProject, err := client.MoveNote(context.Background(), "session", "note-id", "p1")
	if err != nil || inProject.ProjectID == nil || *inProject.ProjectID != "p1" || inProject.Content != "untouched" {
		t.Fatalf("move into project: %+v, %v", inProject, err)
	}
	inInbox, err := client.MoveNote(context.Background(), "session", "note-id", "")
	if err != nil || inInbox.ProjectID != nil || inInbox.Content != "untouched" {
		t.Fatalf("move to Inbox: %+v, %v", inInbox, err)
	}
}

func TestMoveConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"conflict","message":"Archived project."}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.MoveNote(context.Background(), "session", "note-id", "p1")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != "conflict" {
		t.Fatalf("expected conflict: %v", err)
	}
}

func TestArchiveProjectEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("missing Authorization for %s", r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects":
			if r.URL.Query().Get("status") != "archived" || len(r.URL.Query()) != 1 {
				t.Errorf("expected archived status query: %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"id":"p1","name":"Past","archived_at":"2026-09-29T10:00:00Z"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/p1/archive":
			if r.ContentLength > 0 {
				t.Error("archive request must not contain a body")
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/p1/unarchive":
			if r.ContentLength > 0 {
				t.Error("unarchive request must not contain a body")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	projects, err := client.ArchivedProjects(context.Background(), "session")
	if err != nil || len(projects) != 1 || projects[0].ArchivedAt == nil || projects[0].ID != "p1" {
		t.Fatalf("archived projects: %+v, %v", projects, err)
	}
	if err := client.ArchiveProject(context.Background(), "session", "p1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := client.UnarchiveProject(context.Background(), "session", "p1"); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
}
