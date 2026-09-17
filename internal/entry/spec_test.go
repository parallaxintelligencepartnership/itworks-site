package entry

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSpecTableMatchesTheRules keeps docs/SPEC.md honest. The README points
// at that table instead of repeating it, so the table is the one written
// copy of the contract and it must match the code.
func TestSpecTableMatchesTheRules(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "SPEC.md"))
	if err != nil {
		t.Fatalf("read the spec: %v", err)
	}

	rows := map[string]string{}
	inTable := false
	for _, line := range strings.Split(string(spec), "\n") {
		switch {
		case strings.HasPrefix(line, "| Field | Type | Rule |"):
			inTable = true
			continue
		case inTable && !strings.HasPrefix(line, "|"):
			inTable = false
		}
		if !inTable || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 3 {
			t.Fatalf("rules table row is not three cells: %q", line)
		}
		rows[strings.TrimSpace(cells[0])] = strings.TrimSpace(cells[1]) + " " + strings.TrimSpace(cells[2])
	}
	if len(rows) == 0 {
		t.Fatal("docs/SPEC.md has no rules table")
	}

	// Every field of the record is in the table, and nothing else is.
	rt := reflect.TypeOf(Record{})
	fields := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		fields[tag] = true
		if _, ok := rows[tag]; !ok {
			t.Errorf("docs/SPEC.md does not document the %s field", tag)
		}
	}
	for name := range rows {
		if !fields[name] {
			t.Errorf("docs/SPEC.md documents %s, which is not a field of the record", name)
		}
	}

	// Every limit in the table is the limit the code enforces.
	limits := []struct {
		field string
		want  string
	}{
		{"name", fmt.Sprintf("1 to %d characters", maxNameLen)},
		{"summary", fmt.Sprintf("1 to %d characters", maxSummaryLen)},
		{"repo_url", fmt.Sprintf("at most %d characters", maxRepoURLLen)},
		{"audit_date", minAuditYear},
		{"found", fmt.Sprintf("0 to %d", maxCounterVal)},
		{"fixed", fmt.Sprintf("0 to %d", maxCounterVal)},
		{"accepted", fmt.Sprintf("0 to %d", maxCounterVal)},
		{"critical_open", fmt.Sprintf("0 to %d", maxCounterVal)},
		{"critical_accepted", fmt.Sprintf("0 to %d", maxCounterVal)},
	}
	for _, l := range limits {
		if !strings.Contains(rows[l.field], l.want) {
			t.Errorf("docs/SPEC.md rule for %s does not say %q: %q", l.field, l.want, rows[l.field])
		}
	}

	// The cross field rules and the id format are written down too.
	for _, want := range []string{
		"fixed + accepted <= found",
		"critical_open <= found",
		"critical_open + critical_accepted <= found",
		"^[a-z2-7]{12}$",
	} {
		if !strings.Contains(string(spec), want) {
			t.Errorf("docs/SPEC.md does not state the rule %q", want)
		}
	}
}
