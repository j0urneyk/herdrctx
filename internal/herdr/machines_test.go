package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParseMachines(t *testing.T) {
	rows, err := ParseMachines([]byte(`[{"id":"id1","label":"Work","target":"workbox","session":"api","enabled":true,"selected":false},{"id":"id2","label":"Paused","target":"workbox","session":"default","enabled":false,"selected":true}]`))
	if err != nil || len(rows) != 2 || rows[0].Session != "api" || rows[1].Enabled {
		t.Fatalf("%+v %v", rows, err)
	}
	for _, raw := range []string{`null`, `{}`, `[{"id":"x","label":"Bad","target":"-bad","session":"api"}]`, `[{"id":"x","label":"Bad","target":"ok","session":"bad name"}]`} {
		if _, err := ParseMachines([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestMachinesUsesReadOnlyLocalCLI(t *testing.T) {
	for _, version := range []string{"0.8.2", "0.9.0"} {
		c := NewClient(fakeHerdr(t, fmt.Sprintf(`
if [ "$1" = --version ]; then echo 'herdr %s'; exit 0; fi
if [ "$1" = machine ] && [ "$2" = list ] && [ "$3" = --json ] && [ "$#" = 3 ]; then echo '[]'; exit 0; fi
exit 42
`, version)))
		rows, err := c.Machines(context.Background())
		if version == "0.8.2" {
			if err == nil || !strings.Contains(err.Error(), "0.9.0") {
				t.Fatalf("old version: %v", err)
			}
		} else if err != nil || len(rows) != 0 {
			t.Fatalf("list: %v %v", rows, err)
		}
	}
}
