package preferences

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHostChangesPreserveOtherInstances(t *testing.T) {
	s := &HostStore{Path: filepath.Join(t.TempDir(), "hosts.json")}
	a := Host{Label: "One", Destination: "one"}
	b := Host{Label: "Two", Destination: "two"}
	_, err := s.Apply(HostChange{Host: a})
	if err != nil {
		t.Fatal(err)
	}
	other := &HostStore{Path: s.Path}
	_, err = other.Apply(HostChange{Host: b})
	if err != nil {
		t.Fatal(err)
	}
	a.Label = "Renamed"
	d, err := s.Apply(HostChange{Previous: "one", Host: a})
	if err != nil || len(d.Items) != 2 || d.Items[1] != b {
		t.Fatalf("%+v %v", d, err)
	}
	before, _ := os.ReadFile(s.Path)
	_, err = s.Apply(HostChange{Host: Host{Label: "Conflict", Destination: "two"}})
	after, _ := os.ReadFile(s.Path)
	if err == nil || !bytes.Equal(before, after) {
		t.Fatal("conflict overwrote file")
	}
	lock, err := lockFile(s.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	_, err = other.Apply(HostChange{Previous: "one", Remove: true})
	_ = lock.Close()
	if err == nil {
		t.Fatal("concurrent save ignored lock")
	}
	d, err = s.Apply(HostChange{Previous: "one", Remove: true})
	if err != nil || len(d.Items) != 1 || d.Items[0] != b {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestInvalidHostsPreserved(t *testing.T) {
	for _, raw := range []string{`{`, `{"version":2,"hosts":[]}`, `{"version":1,"hosts":[],"secret":"no"}`, `{"version":1,"hosts":[{"label":"Bad","destination":"-oProxyCommand=bad"}]}`} {
		s := &HostStore{Path: filepath.Join(t.TempDir(), "hosts.json")}
		if err := os.WriteFile(s.Path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(HostChange{Host: Host{Label: "Good", Destination: "good"}}); err == nil {
			t.Fatal("invalid accepted")
		}
		after, _ := os.ReadFile(s.Path)
		if string(after) != raw {
			t.Fatal("invalid file overwritten")
		}
	}
}
