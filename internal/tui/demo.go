package tui

import (
	"fmt"
	"time"

	"shortlog-cli/internal/api"
)

// demoData is sample content for the -demo flag and the workspace tests:
// enough projects to scroll the projects panel, an archived tab, a long note
// to scroll the reader, and a project with no notes.
func demoData(now time.Time) workspaceData {
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
	return workspaceData{
		account: api.Account{ID: "demo", Username: "Ari", TimeZone: "Europe/London", CreatedAt: ago(200 * day)},
		sessions: []api.Session{
			{ID: "s1", DeviceLabel: "MacBook Pro", Current: true, LastUsedAt: now},
			{ID: "s2", DeviceLabel: "iPhone", LastUsedAt: ago(2 * day)},
			{ID: "s3", UserAgent: "shortlog-cli/linux", LastUsedAt: ago(35 * day)},
		},
		server:   "demo",
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
