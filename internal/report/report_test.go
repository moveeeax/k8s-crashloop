package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/moveeeax/k8s-crashloop/internal/analyze"
)

func sample() []analyze.Finding {
	return []analyze.Finding{
		{Namespace: "prod", Pod: "api-abc", Container: "api", RestartCount: 12, ExitCode: 137, LastState: "Terminated:OOMKilled", Logs: "panic: nil map"},
	}
}

func TestTable(t *testing.T) {
	var b bytes.Buffer
	Table(&b, sample(), false)
	out := b.String()
	for _, sub := range []string{"NAMESPACE", "api-abc", "137", "OOMKilled", "1 crashlooping", "panic: nil map"} {
		if !strings.Contains(out, sub) {
			t.Errorf("table missing %q\n%s", sub, out)
		}
	}
}

func TestTableEmpty(t *testing.T) {
	var b bytes.Buffer
	Table(&b, nil, false)
	if !strings.Contains(b.String(), "No CrashLoopBackOff") {
		t.Errorf("unexpected: %s", b.String())
	}
}

func TestTableColorHighlightsHighRestarts(t *testing.T) {
	var b bytes.Buffer
	Table(&b, sample(), true)
	if !strings.Contains(b.String(), "\033[31m") {
		t.Errorf("expected red for 12 restarts:\n%q", b.String())
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sample()); err != nil {
		t.Fatal(err)
	}
	var got []analyze.Finding
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 || got[0].ExitCode != 137 {
		t.Errorf("unexpected: %+v", got)
	}
}
