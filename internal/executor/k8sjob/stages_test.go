package k8sjob

import (
	"encoding/json"
	"testing"

	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/progress"
)

func mustPods(t *testing.T, js string) []podDoc {
	t.Helper()
	var l podList
	if err := json.Unmarshal([]byte(js), &l); err != nil {
		t.Fatal(err)
	}
	return l.Items
}

func mustJob(t *testing.T, js string) jobDoc {
	t.Helper()
	var j jobDoc
	if err := json.Unmarshal([]byte(js), &j); err != nil {
		t.Fatal(err)
	}
	return j
}

func statuses(st []progress.Stage) []string {
	out := make([]string, len(st))
	for i, s := range st {
		out[i] = s.Status
	}
	return out
}

func TestJobStages(t *testing.T) {
	const scheduled = `{"items":[{"metadata":{"name":"p","creationTimestamp":"2026-10-22T10:00:00Z"},
		"status":{"phase":"Pending","conditions":[{"type":"PodScheduled","status":"True","lastTransitionTime":"2026-10-22T10:00:02Z"}],
		"containerStatuses":[{"state":{"waiting":{"reason":"ContainerCreating"}}}]}}]}`
	const running = `{"items":[{"metadata":{"name":"p","creationTimestamp":"2026-10-22T10:00:00Z"},
		"status":{"phase":"Running","conditions":[{"type":"PodScheduled","status":"True","lastTransitionTime":"2026-10-22T10:00:02Z"}],
		"containerStatuses":[{"state":{"running":{"startedAt":"2026-10-22T10:00:20Z"}}}]}}]}`
	const exited = `{"items":[{"metadata":{"name":"p","creationTimestamp":"2026-10-22T10:00:00Z"},
		"status":{"phase":"Failed","conditions":[{"type":"PodScheduled","status":"True"}],
		"containerStatuses":[{"state":{"terminated":{"exitCode":1,"startedAt":"2026-10-22T10:00:20Z","finishedAt":"2026-10-22T10:01:00Z"}}}]}}]}`
	none := mustJob(t, `{"status":{}}`)
	complete := mustJob(t, `{"status":{"conditions":[{"type":"Complete","status":"True"}]}}`)

	cases := []struct {
		name  string
		job   jobDoc
		pods  []podDoc
		phase executor.Phase
		want  []string
	}{
		{"no pod yet", none, nil, executor.PhasePending, []string{"queued", "queued", "queued"}},
		{"pulling", none, mustPods(t, scheduled), executor.PhaseScheduled, []string{"success", "running", "queued"}},
		{"running", none, mustPods(t, running), executor.PhaseRunning, []string{"success", "success", "running"}},
		{"complete", complete, mustPods(t, exited), executor.PhaseSucceeded, []string{"success", "success", "success"}},
		{"script failed", none, mustPods(t, exited), executor.PhaseFailed, []string{"success", "success", "failed"}},
		{"pull failed", none, mustPods(t, scheduled), executor.PhaseFailed, []string{"success", "failed", "skipped"}},
		{"cancelled while running", none, mustPods(t, running), executor.PhaseCancelled, []string{"success", "success", "cancelled"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := statuses(jobStages(c.job, c.pods, c.phase))
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("stages = %v, want %v", got, c.want)
				}
			}
		})
	}

	st := jobStages(none, mustPods(t, running), executor.PhaseRunning)
	if st[2].StartedAt == nil || st[1].FinishedAt == nil || !st[1].FinishedAt.Equal(*st[2].StartedAt) {
		t.Fatalf("pull should end when the script starts: %+v", st)
	}
}
