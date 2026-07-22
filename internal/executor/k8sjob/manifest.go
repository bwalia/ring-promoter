package k8sjob

import (
	"sort"
	"strings"

	"github.com/example/ring-promoter/internal/executor"
)

// Labels and annotations stamped on every Job (and its pods) so executions can
// be found again — by the status watcher, by a future restart-reattach sweep,
// and by humans with kubectl.
const (
	labelManagedBy   = "app.kubernetes.io/managed-by"
	managedByValue   = "ring-promoter"
	labelExecutionID = "ring-promoter.io/execution-id"
	labelApp         = "ring-promoter.io/app"
	labelRing        = "ring-promoter.io/ring"

	annotationAction  = "ring-promoter.io/action"
	annotationVersion = "ring-promoter.io/version"

	containerName = "deploy"
)

// The manifest is generated as typed structs and marshalled to JSON (which
// kubectl accepts via `create -f -`) — no template strings, no YAML dependency.

type jobManifest struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   metadata `json:"metadata"`
	Spec       jobSpec  `json:"spec"`
}

type metadata struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type jobSpec struct {
	// BackoffLimit is a pointer so an explicit 0 (no retries) is serialized
	// rather than falling back to the Kubernetes default of 6.
	BackoffLimit            *int32      `json:"backoffLimit,omitempty"`
	ActiveDeadlineSeconds   *int64      `json:"activeDeadlineSeconds,omitempty"`
	TTLSecondsAfterFinished *int32      `json:"ttlSecondsAfterFinished,omitempty"`
	Template                podTemplate `json:"template"`
}

type podTemplate struct {
	Metadata metadata `json:"metadata,omitempty"`
	Spec     podSpec  `json:"spec"`
}

type podSpec struct {
	RestartPolicy      string            `json:"restartPolicy"`
	ServiceAccountName string            `json:"serviceAccountName,omitempty"`
	ImagePullSecrets   []nameRef         `json:"imagePullSecrets,omitempty"`
	NodeSelector       map[string]string `json:"nodeSelector,omitempty"`
	Tolerations        []toleration      `json:"tolerations,omitempty"`
	Affinity           map[string]any    `json:"affinity,omitempty"`
	Containers         []container       `json:"containers"`
}

type nameRef struct {
	Name string `json:"name"`
}

type toleration struct {
	Key      string `json:"key,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
	Effect   string `json:"effect,omitempty"`
}

type container struct {
	Name            string           `json:"name"`
	Image           string           `json:"image"`
	Command         []string         `json:"command,omitempty"`
	Args            []string         `json:"args,omitempty"`
	Env             []envVar         `json:"env,omitempty"`
	EnvFrom         []envFromSource  `json:"envFrom,omitempty"`
	Resources       *resources       `json:"resources,omitempty"`
	SecurityContext *securityContext `json:"securityContext,omitempty"`
}

// securityContext mirrors the container securityContext fields the executor
// exposes. Pointer fields serialize an explicit false/0 while staying omitted
// when unset.
type securityContext struct {
	Privileged             *bool  `json:"privileged,omitempty"`
	RunAsUser              *int64 `json:"runAsUser,omitempty"`
	RunAsGroup             *int64 `json:"runAsGroup,omitempty"`
	RunAsNonRoot           *bool  `json:"runAsNonRoot,omitempty"`
	ReadOnlyRootFilesystem *bool  `json:"readOnlyRootFilesystem,omitempty"`
}

type envVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type envFromSource struct {
	SecretRef    *nameRef `json:"secretRef,omitempty"`
	ConfigMapRef *nameRef `json:"configMapRef,omitempty"`
}

type resources struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// buildManifest turns a Spec into a batch/v1 Job. Every field traces back to
// the Spec; the executor adds only its own identity labels and the
// RP_EXECUTION_ID contract variable.
func buildManifest(spec executor.Spec, name, id string) jobManifest {
	labels := map[string]string{}
	for k, v := range spec.Labels {
		labels[k] = v
	}
	// Identity labels win over user labels: they are what Cancel/Status and a
	// future restart-reattach rely on.
	labels[labelManagedBy] = managedByValue
	labels[labelExecutionID] = id
	labels[labelApp] = sanitizeName(spec.App, 63)
	labels[labelRing] = sanitizeName(spec.Ring, 63)

	annotations := map[string]string{}
	for k, v := range spec.Annotations {
		annotations[k] = v
	}
	if spec.Action != "" {
		annotations[annotationAction] = spec.Action
	}
	if v := spec.Env[executor.EnvVersion]; v != "" {
		annotations[annotationVersion] = v
	}

	backoff := int32(spec.Retries)
	js := jobSpec{
		BackoffLimit: &backoff,
		Template: podTemplate{
			Metadata: metadata{
				// Pods carry the execution id too, so Status and Logs can
				// select them without knowing generated pod names.
				Labels: map[string]string{labelExecutionID: id},
			},
			Spec: podSpec{
				// Retries happen via backoffLimit (fresh pods), never via
				// in-place container restarts.
				RestartPolicy:      "Never",
				ServiceAccountName: spec.ServiceAccount,
				NodeSelector:       spec.NodeSelector,
				Affinity:           spec.Affinity,
				Containers: []container{{
					Name:            containerName,
					Image:           spec.Image,
					Command:         spec.Command,
					Args:            spec.Args,
					Env:             envList(spec.Env, id),
					EnvFrom:         envFromList(spec.EnvFromSecrets, spec.EnvFromConfigMaps),
					Resources:       resourceList(spec.Resources),
					SecurityContext: securityContextOf(spec.SecurityContext),
				}},
			},
		},
	}
	if spec.Timeout > 0 {
		d := int64(spec.Timeout.Seconds())
		js.ActiveDeadlineSeconds = &d
	}
	if spec.TTLAfterFinish > 0 {
		t := int32(spec.TTLAfterFinish.Seconds())
		js.TTLSecondsAfterFinished = &t
	}
	for _, s := range spec.ImagePullSecrets {
		js.Template.Spec.ImagePullSecrets = append(js.Template.Spec.ImagePullSecrets, nameRef{Name: s})
	}
	for _, t := range spec.Tolerations {
		js.Template.Spec.Tolerations = append(js.Template.Spec.Tolerations, toleration(t))
	}

	return jobManifest{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Metadata: metadata{
			Name:        name,
			Namespace:   spec.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: js,
	}
}

// envList renders the environment sorted by name (deterministic manifests
// diff cleanly and test reliably), with the execution id appended per the
// runner contract.
func envList(env map[string]string, id string) []envVar {
	merged := make(map[string]string, len(env)+1)
	for k, v := range env {
		merged[k] = v
	}
	merged[executor.EnvExecutionID] = id

	names := make([]string, 0, len(merged))
	for k := range merged {
		names = append(names, k)
	}
	sort.Strings(names)

	out := make([]envVar, 0, len(names))
	for _, n := range names {
		out = append(out, envVar{Name: n, Value: merged[n]})
	}
	return out
}

func envFromList(secrets, configMaps []string) []envFromSource {
	var out []envFromSource
	for _, s := range secrets {
		out = append(out, envFromSource{SecretRef: &nameRef{Name: s}})
	}
	for _, c := range configMaps {
		out = append(out, envFromSource{ConfigMapRef: &nameRef{Name: c}})
	}
	return out
}

// securityContextOf maps the executor's SecurityContext onto the manifest type,
// or returns nil when unset so the field is omitted entirely.
func securityContextOf(sc *executor.SecurityContext) *securityContext {
	if sc == nil {
		return nil
	}
	return &securityContext{
		Privileged:             sc.Privileged,
		RunAsUser:              sc.RunAsUser,
		RunAsGroup:             sc.RunAsGroup,
		RunAsNonRoot:           sc.RunAsNonRoot,
		ReadOnlyRootFilesystem: sc.ReadOnlyRootFilesystem,
	}
}

func resourceList(r executor.Resources) *resources {
	res := &resources{}
	if r.CPURequest != "" || r.MemoryRequest != "" {
		res.Requests = map[string]string{}
		if r.CPURequest != "" {
			res.Requests["cpu"] = r.CPURequest
		}
		if r.MemoryRequest != "" {
			res.Requests["memory"] = r.MemoryRequest
		}
	}
	if r.CPULimit != "" || r.MemoryLimit != "" {
		res.Limits = map[string]string{}
		if r.CPULimit != "" {
			res.Limits["cpu"] = r.CPULimit
		}
		if r.MemoryLimit != "" {
			res.Limits["memory"] = r.MemoryLimit
		}
	}
	if res.Requests == nil && res.Limits == nil {
		return nil
	}
	return res
}

// jobName builds the Job's name: rp-{app}-{ring}-{suffix}, sanitized to a
// DNS-1123 label and truncated to fit the 63-character limit (the suffix — the
// unique part — is always kept whole).
func jobName(app, ring, id string) string {
	suffix := strings.TrimPrefix(id, "exc-")
	ring = sanitizeName(ring, 12)
	// "rp-" + app + "-" + ring + "-" + suffix must fit in 63.
	maxApp := 63 - len("rp-") - 1 - len(ring) - 1 - len(suffix)
	return "rp-" + sanitizeName(app, maxApp) + "-" + ring + "-" + sanitizeName(suffix, 63)
}

// sanitizeName lowercases s, replaces anything outside [a-z0-9-] with '-',
// collapses runs of '-', trims leading/trailing '-', and truncates to maxLen.
func sanitizeName(s string, maxLen int) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !ok {
			if prevDash {
				continue
			}
			b.WriteByte('-')
			prevDash = true
			continue
		}
		b.WriteRune(r)
		prevDash = false
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > maxLen {
		out = strings.Trim(out[:maxLen], "-")
	}
	if out == "" {
		out = "x"
	}
	return out
}
