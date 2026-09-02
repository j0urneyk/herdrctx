package herdr

import "testing"

func TestParseSessions(t *testing.T) {
	t.Parallel()

	data := []byte(`{"sessions":[{"default":true,"name":"default","running":true,"session_dir":"/tmp/herdr","socket_path":"/tmp/herdr/herdr.sock"}]}`)

	sessions, err := ParseSessions(data)
	if err != nil {
		t.Fatalf("ParseSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}

	session := sessions[0]
	if !session.Default || !session.Running || session.Name != "default" {
		t.Fatalf("unexpected session: %#v", session)
	}
	if session.Status() != "running" {
		t.Fatalf("Status() = %q, want running", session.Status())
	}
	if session.DisplayName() != "default *" {
		t.Fatalf("DisplayName() = %q, want default *", session.DisplayName())
	}
}

func TestParseSessionsRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := ParseSessions([]byte(`not-json`))
	if err == nil {
		t.Fatal("ParseSessions() error = nil, want error")
	}
}

func TestParseSessionsRejectsInvalidSessionNames(t *testing.T) {
	t.Parallel()

	for _, data := range [][]byte{
		[]byte(`{"sessions":[{"name":" work "}]}`),
	} {
		if _, err := ParseSessions(data); err == nil {
			t.Fatalf("ParseSessions(%s) error = nil, want invalid name error", data)
		}
	}
}

func TestParseSessionsRejectsDuplicateSessionNames(t *testing.T) {
	t.Parallel()

	_, err := ParseSessions([]byte(`{"sessions":[{"name":"work","running":false},{"name":"work","running":true}]}`))
	if err == nil {
		t.Fatal("ParseSessions() error = nil, want duplicate name error")
	}
}

func TestParseSessionsAllowsNamesOutsideCreationPolicy(t *testing.T) {
	t.Parallel()

	sessions, err := ParseSessions([]byte(`{"sessions":[{"name":"-work"},{"name":"--help"},{"name":"--json"},{"name":"--session"},{"name":"_work"},{"name":".work"},{"name":"help"}]}`))
	if err != nil {
		t.Fatalf("ParseSessions() error = %v", err)
	}
	names := []string{"-work", "--help", "--json", "--session", "_work", ".work", "help"}
	if len(sessions) != len(names) {
		t.Fatalf("sessions = %#v, want all existing sessions", sessions)
	}
	for i, name := range names {
		if sessions[i].Name != name {
			t.Fatalf("session name = %q, want %q", sessions[i].Name, name)
		}
	}
}

func TestParseErrorResponse(t *testing.T) {
	t.Parallel()

	code, message := parseErrorResponse([]byte(`{"error":{"code":"session_delete_failed","message":"cannot delete default"}}`))
	if code != "session_delete_failed" || message != "cannot delete default" {
		t.Fatalf("parseErrorResponse() = %q, %q", code, message)
	}
}

func TestValidateSessionName(t *testing.T) {
	t.Parallel()

	name, err := ValidateSessionName("work-1_2.3")
	if err != nil {
		t.Fatalf("ValidateSessionName() error = %v", err)
	}
	if name != "work-1_2.3" {
		t.Fatalf("name = %q, want work-1_2.3", name)
	}
}

func TestValidateSessionNameRejectsInvalidNames(t *testing.T) {
	t.Parallel()

	longName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, name := range []string{"", "   ", " work", "work ", ".", "..", "with space", "with/slash", longName, "한글"} {
		if _, err := ValidateSessionName(name); err == nil {
			t.Fatalf("ValidateSessionName(%q) error = nil, want error", name)
		}
	}
}

func TestValidateSessionNameAllowsLeadingHyphenNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"-", "-work", "--help"} {
		got, err := ValidateSessionName(name)
		if err != nil {
			t.Fatalf("ValidateSessionName(%q) error = %v", name, err)
		}
		if got != name {
			t.Fatalf("ValidateSessionName(%q) = %q, want unchanged", name, got)
		}
	}
}
