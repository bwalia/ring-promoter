package k8sjob

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/executor"
)

// fakeRunner scripts kubectl: it records created manifests and deletions, and
// serves successive job/pod JSON documents (the last repeats).
type fakeRunner struct {
	mu         sync.Mutex
	created    [][]byte
	deletes    [][]string
	streamArgs [][]string
	jobJSON    []string
	jobIdx     int
	podsJSON   []string
	podsIdx    int
	logs       string
	outputErr  error
}

func (f *fakeRunner) output(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.outputErr != nil {
		return nil, f.outputErr
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "create"):
		f.created = append(f.created, stdin)
		return []byte("job created"), nil
	case strings.Contains(joined, "delete"):
		f.deletes = append(f.deletes, args)
		return []byte("deleted"), nil
	case strings.Contains(joined, "get job"):
		out := f.jobJSON[min(f.jobIdx, len(f.jobJSON)-1)]
		f.jobIdx++
		return []byte(out), nil
	case strings.Contains(joined, "get pods"):
		if len(f.podsJSON) == 0 {
			return []byte(`{"items":[]}`), nil
		}
		out := f.podsJSON[min(f.podsIdx, len(f.podsJSON)-1)]
		f.podsIdx++
		return []byte(out), nil
	}
	return nil, fmt.Errorf("fakeRunner: unexpected command %q", joined)
}

func (f *fakeRunner) stream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streamArgs = append(f.streamArgs, args)
	return io.NopCloser(strings.NewReader(f.logs)), nil
}

func testExecutor(f *fakeRunner) *Executor {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Executor{log: quiet, run: f, retryBackoff: time.Millisecond}
}

func fullSpec() executor.Spec {
	return executor.Spec{
		App: "MyApp", Ring: "test", Action: "promote",
		Image:   "ghcr.io/bwalia/deploy-runner:v1",
		Command: []string{"/scripts/deploy.sh"},
		Args:    []string{"--mode", "full"},
		Env: map[string]string{
			executor.EnvApp:     "MyApp",
			executor.EnvRing:    "test",
			executor.EnvVersion: "v1.4.2",
			"CUSTOM":            "yes",
		},
		EnvFromSecrets:    []string{"myapp-deploy-credentials"},
		EnvFromConfigMaps: []string{"myapp-deploy-config"},
		ImagePullSecrets:  []string{"ghcr-pull"},
		Namespace:         "ring-exec",
		ServiceAccount:    "ring-deploy-job",
		Resources: executor.Resources{
			CPURequest: "250m", MemoryRequest: "256Mi",
			CPULimit: "1", MemoryLimit: "512Mi",
		},
		NodeSelector:   map[string]string{"kubernetes.io/arch": "amd64"},
		Tolerations:    []executor.Toleration{{Key: "ci", Operator: "Exists", Effect: "NoSchedule"}},
		Timeout:        30 * time.Minute,
		Retries:        2,
		TTLAfterFinish: time.Hour,
		Labels:         map[string]string{"team": "platform"},
		Annotations:    map[string]string{"note": "hello"},
	}
}

func TestStart_BuildsManifest(t *testing.T) {
	f := &fakeRunner{}
	ex, err := testExecutor(f).Start(context.Background(), fullSpec())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(f.created) != 1 {
		t.Fatalf("expected one create, got %d", len(f.created))
	}

	var m jobManifest
	if err := json.Unmarshal(f.created[0], &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}

	if m.APIVersion != "batch/v1" || m.Kind != "Job" {
		t.Fatalf("wrong GVK: %s %s", m.APIVersion, m.Kind)
	}
	if !strings.HasPrefix(m.Metadata.Name, "rp-myapp-test-") {
		t.Fatalf("job name = %q", m.Metadata.Name)
	}
	if len(m.Metadata.Name) > 63 {
		t.Fatalf("job name too long: %d", len(m.Metadata.Name))
	}
	if m.Metadata.Namespace != "ring-exec" {
		t.Fatalf("namespace = %q", m.Metadata.Namespace)
	}
	if m.Metadata.Labels[labelExecutionID] != ex.ID() {
		t.Fatalf("execution-id label = %q, want %q", m.Metadata.Labels[labelExecutionID], ex.ID())
	}
	if m.Metadata.Labels[labelManagedBy] != managedByValue || m.Metadata.Labels["team"] != "platform" {
		t.Fatalf("labels = %+v", m.Metadata.Labels)
	}
	if m.Metadata.Annotations[annotationAction] != "promote" || m.Metadata.Annotations[annotationVersion] != "v1.4.2" {
		t.Fatalf("annotations = %+v", m.Metadata.Annotations)
	}
	if m.Spec.BackoffLimit == nil || *m.Spec.BackoffLimit != 2 {
		t.Fatalf("backoffLimit = %v", m.Spec.BackoffLimit)
	}
	if m.Spec.ActiveDeadlineSeconds == nil || *m.Spec.ActiveDeadlineSeconds != 1800 {
		t.Fatalf("activeDeadlineSeconds = %v", m.Spec.ActiveDeadlineSeconds)
	}
	if m.Spec.TTLSecondsAfterFinished == nil || *m.Spec.TTLSecondsAfterFinished != 3600 {
		t.Fatalf("ttlSecondsAfterFinished = %v", m.Spec.TTLSecondsAfterFinished)
	}

	ps := m.Spec.Template.Spec
	if ps.RestartPolicy != "Never" {
		t.Fatalf("restartPolicy = %q", ps.RestartPolicy)
	}
	if ps.ServiceAccountName != "ring-deploy-job" {
		t.Fatalf("serviceAccountName = %q", ps.ServiceAccountName)
	}
	if len(ps.ImagePullSecrets) != 1 || ps.ImagePullSecrets[0].Name != "ghcr-pull" {
		t.Fatalf("imagePullSecrets = %+v", ps.ImagePullSecrets)
	}
	if ps.NodeSelector["kubernetes.io/arch"] != "amd64" {
		t.Fatalf("nodeSelector = %+v", ps.NodeSelector)
	}
	if len(ps.Tolerations) != 1 || ps.Tolerations[0].Key != "ci" {
		t.Fatalf("tolerations = %+v", ps.Tolerations)
	}
	if m.Spec.Template.Metadata.Labels[labelExecutionID] != ex.ID() {
		t.Fatal("pod template must carry the execution-id label")
	}

	c := ps.Containers[0]
	if c.Image != "ghcr.io/bwalia/deploy-runner:v1" || c.Command[0] != "/scripts/deploy.sh" || c.Args[1] != "full" {
		t.Fatalf("container = %+v", c)
	}
	if c.Resources == nil || c.Resources.Requests["cpu"] != "250m" || c.Resources.Limits["memory"] != "512Mi" {
		t.Fatalf("resources = %+v", c.Resources)
	}
	if len(c.EnvFrom) != 2 || c.EnvFrom[0].SecretRef.Name != "myapp-deploy-credentials" || c.EnvFrom[1].ConfigMapRef.Name != "myapp-deploy-config" {
		t.Fatalf("envFrom = %+v", c.EnvFrom)
	}

	// Env must be sorted by name and include the execution id.
	got := map[string]string{}
	for i, e := range c.Env {
		got[e.Name] = e.Value
		if i > 0 && c.Env[i-1].Name > e.Name {
			t.Fatalf("env not sorted: %q before %q", c.Env[i-1].Name, e.Name)
		}
	}
	if got[executor.EnvExecutionID] != ex.ID() || got["CUSTOM"] != "yes" || got[executor.EnvVersion] != "v1.4.2" {
		t.Fatalf("env = %+v", got)
	}
}

func TestStart_RequiresImageAndNamespace(t *testing.T) {
	e := testExecutor(&fakeRunner{})
	if _, err := e.Start(context.Background(), executor.Spec{App: "a", Namespace: "ns"}); err == nil || !strings.Contains(err.Error(), "image") {
		t.Fatalf("expected image error, got %v", err)
	}
	if _, err := e.Start(context.Background(), executor.Spec{App: "a", Image: "img"}); err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

// ---- status mapping ----

func termPod(created time.Time, exitCode int, reason string) podDoc {
	var p podDoc
	p.Metadata.Name = "p1"
	p.Metadata.CreationTimestamp = created
	p.Status.Phase = "Failed"
	p.Status.ContainerStatuses = []containerStatus{{}}
	p.Status.ContainerStatuses[0].State.Terminated = &struct {
		ExitCode int    `json:"exitCode"`
		Reason   string `json:"reason"`
		Message  string `json:"message"`
	}{ExitCode: exitCode, Reason: reason}
	return p
}

func waitingPod(created time.Time, reason, message string) podDoc {
	var p podDoc
	p.Metadata.Name = "p1"
	p.Metadata.CreationTimestamp = created
	p.Status.Phase = "Pending"
	p.Status.ContainerStatuses = []containerStatus{{}}
	p.Status.ContainerStatuses[0].State.Waiting = &struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}{Reason: reason, Message: message}
	return p
}

func jobWithCondition(typ, reason string, failed int) jobDoc {
	var j jobDoc
	j.Status.Failed = failed
	j.Status.Conditions = []jobCondition{{Type: typ, Status: "True", Reason: reason}}
	return j
}

func TestMapStatus(t *testing.T) {
	now := time.Now()
	spec := fullSpec()

	tests := []struct {
		name      string
		job       jobDoc
		pods      []podDoc
		wantPhase executor.Phase
		wantIn    string // substring of Message
		wantExit  *int
	}{
		{
			name:      "complete condition wins",
			job:       jobWithCondition("Complete", "", 0),
			wantPhase: executor.PhaseSucceeded,
			wantExit:  intp(0),
		},
		{
			name:      "backoff limit exceeded carries last exit code",
			job:       jobWithCondition("Failed", "BackoffLimitExceeded", 3),
			pods:      []podDoc{termPod(now, 3, "Error")},
			wantPhase: executor.PhaseFailed,
			wantIn:    "failed after 3 attempt(s): last exit code 3",
			wantExit:  intp(3),
		},
		{
			name:      "deadline exceeded is a timeout",
			job:       jobWithCondition("Failed", "DeadlineExceeded", 1),
			wantPhase: executor.PhaseTimedOut,
			wantIn:    "timed out after 30m",
		},
		{
			name:      "oom is named",
			job:       jobWithCondition("Failed", "BackoffLimitExceeded", 1),
			pods:      []podDoc{termPod(now, 137, "OOMKilled")},
			wantPhase: executor.PhaseFailed,
			wantIn:    "out of memory (OOMKilled), memory limit was 512Mi",
			wantExit:  intp(137),
		},
		{
			name:      "fresh image pull backoff is pending",
			job:       jobDoc{},
			pods:      []podDoc{waitingPod(now, "ImagePullBackOff", "pull failed")},
			wantPhase: executor.PhasePending,
			wantIn:    "pulling image",
		},
		{
			name:      "stale image pull backoff fails",
			job:       jobDoc{},
			pods:      []podDoc{waitingPod(now.Add(-3*time.Minute), "ErrImagePull", "manifest unknown")},
			wantPhase: executor.PhaseFailed,
			wantIn:    "image pull failed for ghcr.io/bwalia/deploy-runner:v1: manifest unknown",
		},
		{
			name: "unschedulable pod says why",
			job:  jobDoc{},
			pods: func() []podDoc {
				var p podDoc
				p.Metadata.CreationTimestamp = now
				p.Status.Phase = "Pending"
				p.Status.Conditions = []podCondition{{Type: "PodScheduled", Status: "False", Reason: "Unschedulable", Message: "0/3 nodes match selector"}}
				return []podDoc{p}
			}(),
			wantPhase: executor.PhasePending,
			wantIn:    "cannot schedule pod: 0/3 nodes match selector",
		},
		{
			name: "running pod is running",
			job:  jobDoc{},
			pods: func() []podDoc {
				var p podDoc
				p.Metadata.CreationTimestamp = now
				p.Status.Phase = "Running"
				return []podDoc{p}
			}(),
			wantPhase: executor.PhaseRunning,
		},
		{
			name: "failed attempt with retries left is retrying",
			job:  func() jobDoc { var j jobDoc; j.Status.Failed = 1; return j }(),
			pods: []podDoc{termPod(now, 3, "Error")},

			wantPhase: executor.PhaseRetrying,
			wantIn:    "retrying",
			wantExit:  intp(3),
		},
		{
			name:      "no pods yet",
			job:       jobDoc{},
			wantPhase: executor.PhasePending,
			wantIn:    "waiting for a pod",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := mapStatus(spec, tc.job, tc.pods, now)
			if st.Phase != tc.wantPhase {
				t.Fatalf("phase = %s, want %s (msg %q)", st.Phase, tc.wantPhase, st.Message)
			}
			if tc.wantIn != "" && !strings.Contains(st.Message, tc.wantIn) {
				t.Fatalf("message = %q, want substring %q", st.Message, tc.wantIn)
			}
			if tc.wantExit != nil {
				if st.ExitCode == nil || *st.ExitCode != *tc.wantExit {
					t.Fatalf("exit code = %v, want %d", st.ExitCode, *tc.wantExit)
				}
			}
		})
	}
}

func intp(n int) *int { return &n }

// ---- execution end to end against the fake runner ----

func TestExecution_StatusLifecycle(t *testing.T) {
	f := &fakeRunner{
		jobJSON: []string{
			`{"status":{"active":1}}`,
			`{"status":{"succeeded":1,"conditions":[{"type":"Complete","status":"True"}]}}`,
		},
		podsJSON: []string{
			`{"items":[{"metadata":{"name":"rp-a-int-x-z9","creationTimestamp":"2026-07-17T12:00:00Z"},"spec":{"nodeName":"k3s-worker-2"},"status":{"phase":"Running"}}]}`,
		},
	}
	ex, err := testExecutor(f).Start(context.Background(), minimalSpec())
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	st, err := ex.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Phase != executor.PhaseRunning {
		t.Fatalf("phase = %s, want running", st.Phase)
	}
	if st.Details["node"] != "k3s-worker-2" || st.Details["pod_name"] == "" {
		t.Fatalf("details = %+v", st.Details)
	}
	if st.Details["job_name"] == "" || st.Details["namespace"] != "ring-exec" {
		t.Fatalf("details = %+v", st.Details)
	}

	st, err = ex.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Phase != executor.PhaseSucceeded {
		t.Fatalf("phase = %s, want succeeded", st.Phase)
	}
}

func minimalSpec() executor.Spec {
	return executor.Spec{App: "a", Ring: "int", Image: "img:v1", Namespace: "ring-exec"}
}

func TestExecution_CancelDeletesJob(t *testing.T) {
	f := &fakeRunner{}
	ex, err := testExecutor(f).Start(context.Background(), minimalSpec())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := ex.Cancel(context.Background()); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if len(f.deletes) != 1 {
		t.Fatalf("expected one delete, got %+v", f.deletes)
	}
	joined := strings.Join(f.deletes[0], " ")
	for _, want := range []string{"delete job", "--ignore-not-found", "--cascade=foreground"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("delete args missing %q: %s", want, joined)
		}
	}
}

func TestExecution_LogsArgs(t *testing.T) {
	f := &fakeRunner{logs: "line1\n"}
	ex, err := testExecutor(f).Start(context.Background(), minimalSpec())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	rc, err := ex.Logs(context.Background(), executor.LogOptions{
		Follow: true, Timestamps: true, SinceTime: "2026-07-17T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	defer rc.Close()

	joined := strings.Join(f.streamArgs[0], " ")
	for _, want := range []string{"logs job/", "--follow", "--timestamps", "--since-time=2026-07-17T12:00:00Z"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("log args missing %q: %s", want, joined)
		}
	}
	body, _ := io.ReadAll(rc)
	if string(body) != "line1\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestJobName(t *testing.T) {
	name := jobName("My_Weird App!", "prod", "exc-9f3a1c2e")
	if name != "rp-my-weird-app-prod-9f3a1c2e" {
		t.Fatalf("jobName = %q", name)
	}
	long := jobName(strings.Repeat("a", 100), "test", "exc-9f3a1c2e")
	if len(long) > 63 {
		t.Fatalf("jobName too long: %d %q", len(long), long)
	}
	if !strings.HasSuffix(long, "-9f3a1c2e") {
		t.Fatalf("unique suffix lost: %q", long)
	}
}
