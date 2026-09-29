package tui

import (
	"context"
	"fmt"
	"time"

	"shortlog-cli/internal/api"
)

// demoAPI serves sample content for the -demo flag and the workspace tests,
// through the same interface the workspace uses for the real server: enough
// projects to scroll the projects panel, an archived tab, a long note to
// scroll the reader, and a project with no notes.
type demoAPI struct {
	account  api.Account
	sessions []api.Session
	active   []api.Project
	archived []api.Project
	inbox    []api.Note
	notes    map[string][]api.Note // by project ID
	nextID   int
}

func (d *demoAPI) Me(context.Context, string) (api.Account, error) { return d.account, nil }
func (d *demoAPI) Sessions(context.Context, string) ([]api.Session, error) {
	return d.sessions, nil
}
func (d *demoAPI) Projects(context.Context, string) ([]api.Project, error) { return d.active, nil }
func (d *demoAPI) ArchivedProjects(context.Context, string) ([]api.Project, error) {
	return d.archived, nil
}
func (d *demoAPI) Inbox(context.Context, string) (api.NotesPage, error) {
	return api.NotesPage{Items: d.inbox}, nil
}
func (d *demoAPI) InboxPage(context.Context, string, string) (api.NotesPage, error) {
	return api.NotesPage{}, nil
}
func (d *demoAPI) ProjectNotes(_ context.Context, _, id string) (api.NotesPage, error) {
	return api.NotesPage{Items: d.notes[id]}, nil
}
func (d *demoAPI) ProjectNotesPage(context.Context, string, string, string) (api.NotesPage, error) {
	return api.NotesPage{}, nil
}

func newDemoAPI(now time.Time) *demoAPI {
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour
	archived := ago(40 * day)
	names := []string{"Work", "Reading list", "Home renovation", "Shortlog", "Travel: Lisbon", "Recipes", "Garden", "Taxes 2026", "Music practice", "Health", "Gift ideas"}
	var active []api.Project
	for i, name := range names {
		active = append(active, api.Project{ID: fmt.Sprintf("p%d", i+1), Name: name})
	}
	note := func(id, content string, age time.Duration) api.Note {
		return api.Note{ID: id, Content: content, CreatedAt: ago(age)}
	}
	return &demoAPI{
		nextID:  1,
		account: api.Account{ID: "demo", Username: "Ari", TimeZone: "Europe/London", CreatedAt: ago(200 * day)},
		sessions: []api.Session{
			{ID: "s1", DeviceLabel: "MacBook Pro", Current: true, LastUsedAt: now},
			{ID: "s2", DeviceLabel: "iPhone", LastUsedAt: ago(2 * day)},
			{ID: "s3", UserAgent: "shortlog-cli/linux", LastUsedAt: ago(35 * day)},
		},
		active:   active,
		archived: []api.Project{{ID: "a1", Name: "Wedding planning", ArchivedAt: &archived}, {ID: "a2", Name: "Old job", ArchivedAt: &archived}},
		inbox: []api.Note{
			note("i1", "Call the bank about the card\nAsk about the foreign transaction fee, and whether the old card stays active until the new one arrives.", 2*time.Hour),
			note("i2", "Buy milk, eggs, and coffee beans", 5*time.Hour),
			note("i3", "Idea: a weekly review note that links the week's highlights", day+3*time.Hour),
			note("i4", "Book the dentist for October", 3*day),
			note("i5", "Podcast recommendation from Sam: \"The Knowledge Project\"", 6*day),
		},
		notes: map[string][]api.Note{
			"p1": {
				note("w1", "Quarterly planning\n\nGoals for Q4:\n1. Ship the new onboarding flow.\n2. Cut the p95 API latency below 200 ms.\n3. Hire two engineers for the platform team.\n\nRisks:\n- The design review is still pending, and the onboarding flow depends on it.\n- Latency work competes with feature work for the same people.\n- Hiring takes longer than a quarter more often than not.\n\nNext steps:\n- Schedule the design review this week.\n- Profile the three slowest endpoints.\n- Draft the job descriptions with the team leads.\n\nOpen questions:\n- Do we freeze features in December?\n- Who owns the latency dashboard?", 4*time.Hour),
				note("w2", "1:1 notes with Priya\nTalked about growth goals and the conference budget.", 2*day),
				note("w3", "Remember to submit the expense report", 4*day),
			},
			"p2": {
				note("r1", "The Pragmatic Programmer, chapter 7", 3*day),
				note("r2", "Designing Data-Intensive Applications: re-read the chapter on replication", 9*day),
			},
			"p4": {
				note("s1", "Workspace redesign: lazygit-style panels, Inbox as a tab in the notes panel", time.Hour),
			},
			"a1": {note("x1", "Venue shortlist and catering quotes", 60*day)},
		},
	}
}

// The demo's changes live in memory for the run, so every workspace action
// can be tried without a server.

func (d *demoAPI) id(prefix string) string {
	d.nextID++
	return fmt.Sprintf("%s-demo-%d", prefix, d.nextID)
}

func (d *demoAPI) CreateInboxNote(_ context.Context, _, content string) (api.Note, error) {
	note := api.Note{ID: d.id("n"), Content: content, CreatedAt: time.Now()}
	d.inbox = append([]api.Note{note}, d.inbox...)
	return note, nil
}

func (d *demoAPI) CreateProjectNote(_ context.Context, _, projectID, content string) (api.Note, error) {
	id := projectID
	note := api.Note{ID: d.id("n"), Content: content, ProjectID: &id, CreatedAt: time.Now()}
	d.notes[projectID] = append([]api.Note{note}, d.notes[projectID]...)
	return note, nil
}

// find returns the list holding a note, by source, and its index.
func (d *demoAPI) find(id string) (string, int) {
	for i, note := range d.inbox {
		if note.ID == id {
			return inboxSource, i
		}
	}
	for source, notes := range d.notes {
		for i, note := range notes {
			if note.ID == id {
				return source, i
			}
		}
	}
	return "", -1
}

func (d *demoAPI) UpdateNote(_ context.Context, _, id, content string) (api.Note, error) {
	source, i := d.find(id)
	if i < 0 {
		return api.Note{}, &api.Error{Status: 404, Code: "not_found"}
	}
	if source == inboxSource {
		d.inbox[i].Content = content
		return d.inbox[i], nil
	}
	d.notes[source][i].Content = content
	return d.notes[source][i], nil
}

func (d *demoAPI) remove(id string) (api.Note, bool) {
	source, i := d.find(id)
	if i < 0 {
		return api.Note{}, false
	}
	if source == inboxSource {
		note := d.inbox[i]
		d.inbox = append(d.inbox[:i:i], d.inbox[i+1:]...)
		return note, true
	}
	note := d.notes[source][i]
	d.notes[source] = append(d.notes[source][:i:i], d.notes[source][i+1:]...)
	return note, true
}

func (d *demoAPI) MoveNote(_ context.Context, _, id, projectID string) (api.Note, error) {
	note, ok := d.remove(id)
	if !ok {
		return api.Note{}, &api.Error{Status: 404, Code: "not_found"}
	}
	if projectID == "" {
		note.ProjectID = nil
		d.inbox = insertNote(d.inbox, note)
		return note, nil
	}
	note.ProjectID = &projectID
	d.notes[projectID] = insertNote(d.notes[projectID], note)
	return note, nil
}

func (d *demoAPI) DeleteNote(_ context.Context, _, id string) error {
	if _, ok := d.remove(id); !ok {
		return &api.Error{Status: 404, Code: "not_found"}
	}
	return nil
}

func (d *demoAPI) CreateProject(_ context.Context, _, name string) (api.Project, error) {
	p := api.Project{ID: d.id("p"), Name: name}
	d.active = append([]api.Project{p}, d.active...)
	return p, nil
}

func (d *demoAPI) ArchiveProject(_ context.Context, _, id string) error {
	for i, p := range d.active {
		if p.ID == id {
			now := time.Now()
			p.ArchivedAt = &now
			d.active = append(d.active[:i:i], d.active[i+1:]...)
			d.archived = append([]api.Project{p}, d.archived...)
			return nil
		}
	}
	return &api.Error{Status: 404, Code: "not_found"}
}

func (d *demoAPI) UnarchiveProject(_ context.Context, _, id string) error {
	for i, p := range d.archived {
		if p.ID == id {
			p.ArchivedAt = nil
			d.archived = append(d.archived[:i:i], d.archived[i+1:]...)
			d.active = append([]api.Project{p}, d.active...)
			return nil
		}
	}
	return &api.Error{Status: 404, Code: "not_found"}
}

func (d *demoAPI) UpdateProfile(_ context.Context, _, name, zone string) (api.Account, error) {
	d.account.Username, d.account.TimeZone = name, zone
	return d.account, nil
}

// The demo's sessions and deletion succeed without effect; the root starts
// the demo over when one of them ends the session.
func (d *demoAPI) RequestAccountDeletion(context.Context, string) (time.Time, error) {
	return time.Now().Add(30 * 24 * time.Hour), nil
}

func (d *demoAPI) Logout(context.Context, string) error            { return nil }
func (d *demoAPI) RevokeAllSessions(context.Context, string) error { return nil }

func (d *demoAPI) RevokeSession(_ context.Context, _, id string) error {
	for i, s := range d.sessions {
		if s.ID == id {
			d.sessions = append(d.sessions[:i:i], d.sessions[i+1:]...)
			return nil
		}
	}
	return &api.Error{Status: 404, Code: "not_found"}
}
