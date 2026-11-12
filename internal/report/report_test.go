package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func restartSample(now time.Time) []analyze.Restart {
	fin := now.Add(-90 * time.Minute)
	return []analyze.Restart{
		{Namespace: "prod", Pod: "api-1", Container: "app", Owner: "Deployment/api", Node: "n1", RestartCount: 12, Cause: analyze.CauseOOMKilled, Reason: "OOMKilled", ExitCode: 137, FinishedAt: &fin, Waiting: "CrashLoopBackOff"},
		{Namespace: "prod", Pod: "api-1", Container: "migrate", Init: true, Owner: "Deployment/api", RestartCount: 1, Cause: analyze.CauseUnknown},
	}
}

func TestRestartsTable(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	var b bytes.Buffer
	Restarts(&b, restartSample(now), false, false, now)
	out := b.String()
	for _, sub := range []string{"CAUSE", "OOMKilled", "137", "1h", "migrate (init)", "2 restarted container(s)"} {
		if !strings.Contains(out, sub) {
			t.Errorf("table missing %q\n%s", sub, out)
		}
	}
	if strings.Contains(out, "Deployment/api") {
		t.Errorf("owner only in wide output:\n%s", out)
	}

	b.Reset()
	Restarts(&b, restartSample(now), true, false, now)
	for _, sub := range []string{"OWNER", "Deployment/api", "n1", "2026-11-05T10:30:00Z", "CrashLoopBackOff"} {
		if !strings.Contains(b.String(), sub) {
			t.Errorf("wide missing %q\n%s", sub, b.String())
		}
	}
}

func TestRestartsEmpty(t *testing.T) {
	var b bytes.Buffer
	Restarts(&b, nil, false, false, time.Now())
	Owners(&b, nil, false, false, time.Now())
	if strings.Count(b.String(), "No restarted containers") != 2 {
		t.Errorf("unexpected: %s", b.String())
	}
	b.Reset()
	if err := RestartsJSON(&b, nil); err != nil || strings.TrimSpace(b.String()) != "[]" {
		t.Errorf("want [], got %q (%v)", b.String(), err)
	}
}

func TestOwnersTable(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	owners := analyze.ByOwner(restartSample(now))
	var b bytes.Buffer
	Owners(&b, owners, true, true, now)
	out := b.String()
	for _, sub := range []string{"Deployment/api", "OOMKilled:1,Unknown:1", "\033[31m13", "FINISHED", "1 workload(s)"} {
		if !strings.Contains(out, sub) {
			t.Errorf("owners table missing %q\n%q", sub, out)
		}
	}
	b.Reset()
	if err := OwnersJSON(&b, owners); err != nil {
		t.Fatal(err)
	}
	var got []analyze.OwnerRestarts
	if err := json.Unmarshal(b.Bytes(), &got); err != nil || len(got) != 1 || got[0].Restarts != 13 {
		t.Errorf("unexpected JSON %s (%v)", b.String(), err)
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		-time.Second: "0s", 45 * time.Second: "45s", 12 * time.Minute: "12m", 30 * time.Hour: "30h", 72 * time.Hour: "3d",
	} {
		t0 := now.Add(-d)
		if got := age(&t0, now); got != want {
			t.Errorf("age(%v) = %s, want %s", d, got, want)
		}
	}
	if age(nil, now) != "-" {
		t.Error("nil age")
	}
}
