package k8sjob

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/example/ring-promoter/internal/executor"
)

// imagePullGrace is how long a pod may sit in ErrImagePull/ImagePullBackOff
// before the execution is failed outright. Registries blip; a missing image
// does not fix itself — the grace period separates the two without waiting for
// the Job's full deadline.
const imagePullGrace = 2 * time.Minute

// The subset of the Job and Pod status documents we consume, as returned by
// `kubectl get ... -o json`.

type jobDoc struct {
	Status struct {
		Active     int            `json:"active"`
		Succeeded  int            `json:"succeeded"`
		Failed     int            `json:"failed"`
		Conditions []jobCondition `json:"conditions"`
	} `json:"status"`
}

type jobCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type podList struct {
	Items []podDoc `json:"items"`
}

type podDoc struct {
	Metadata struct {
		Name              string    `json:"name"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		Conditions        []podCondition    `json:"conditions"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type podCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type containerStatus struct {
	State     containerState `json:"state"`
	LastState containerState `json:"lastState"`
}

type containerState struct {
	Waiting *struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"waiting"`
	Terminated *struct {
		ExitCode int    `json:"exitCode"`
		Reason   string `json:"reason"`
		Message  string `json:"message"`
	} `json:"terminated"`
}

// mapStatus translates raw Job + Pod state into the backend-agnostic Status,
// turning Kubernetes signals (ImagePullBackOff, OOMKilled, DeadlineExceeded,
// BackoffLimitExceeded, exit codes) into messages a person can act on.
func mapStatus(spec executor.Spec, job jobDoc, pods []podDoc, now time.Time) executor.Status {
	st := executor.Status{Details: map[string]string{
		"retries": strconv.Itoa(job.Status.Failed),
	}}
	pod := newestPod(pods)
	if pod != nil {
		st.Details["pod_name"] = pod.Metadata.Name
		if pod.Spec.NodeName != "" {
			st.Details["node"] = pod.Spec.NodeName
		}
	}

	// Terminal Job conditions first: they are authoritative.
	for _, c := range job.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case "Complete":
			st.Phase = executor.PhaseSucceeded
			zero := 0
			st.ExitCode = &zero
			return st
		case "Failed":
			exit, desc := lastTermination(pods, spec)
			st.ExitCode = exit
			switch c.Reason {
			case "DeadlineExceeded":
				st.Phase = executor.PhaseTimedOut
				if spec.Timeout > 0 {
					st.Message = fmt.Sprintf("timed out after %s", spec.Timeout)
				} else {
					st.Message = "timed out (job deadline exceeded)"
				}
			case "BackoffLimitExceeded":
				st.Phase = executor.PhaseFailed
				attempts := job.Status.Failed
				if attempts < 1 {
					attempts = spec.Retries + 1
				}
				st.Message = fmt.Sprintf("deploy script failed after %d attempt(s)", attempts)
				if desc != "" {
					st.Message += ": " + desc
				}
			default:
				st.Phase = executor.PhaseFailed
				st.Message = firstNonEmpty(c.Message, c.Reason, "job failed")
				if desc != "" {
					st.Message += " (" + desc + ")"
				}
			}
			return st
		}
	}

	// Not terminal yet: derive the phase from the newest pod.
	if pod == nil {
		st.Phase = executor.PhasePending
		st.Message = "waiting for a pod to be created"
		return st
	}

	// Container-level early failures that would otherwise idle until the
	// deadline.
	if w := waitingReason(pod); w != nil {
		switch w.Reason {
		case "ErrImagePull", "ImagePullBackOff":
			detail := firstNonEmpty(w.Message, w.Reason)
			if now.Sub(pod.Metadata.CreationTimestamp) > imagePullGrace {
				st.Phase = executor.PhaseFailed
				st.Message = fmt.Sprintf("image pull failed for %s: %s", spec.Image, detail)
			} else {
				st.Phase = executor.PhasePending
				st.Message = fmt.Sprintf("pulling image %s (%s)", spec.Image, w.Reason)
			}
			return st
		case "CrashLoopBackOff":
			_, desc := lastTermination(pods, spec)
			st.Phase = executor.PhaseRetrying
			st.Message = "container crashing repeatedly"
			if desc != "" {
				st.Message += ": " + desc
			}
			return st
		}
	}

	switch pod.Status.Phase {
	case "Pending":
		if c := findPodCondition(pod, "PodScheduled"); c != nil && c.Status == "False" {
			st.Phase = executor.PhasePending
			st.Message = "cannot schedule pod: " + firstNonEmpty(c.Message, c.Reason) +
				" — check node selectors and resource requests"
		} else if c != nil && c.Status == "True" {
			st.Phase = executor.PhaseScheduled
			st.Message = "pod scheduled" + nodeSuffix(pod)
		} else {
			st.Phase = executor.PhasePending
		}
	case "Running":
		if job.Status.Failed > 0 {
			st.Phase = executor.PhaseRetrying
			st.Message = fmt.Sprintf("attempt %d running (%d failed so far)",
				job.Status.Failed+1, job.Status.Failed)
		} else {
			st.Phase = executor.PhaseRunning
		}
	case "Succeeded":
		// The Job controller has not stamped Complete yet; report running so
		// success is only ever declared from the authoritative condition.
		st.Phase = executor.PhaseRunning
		st.Message = "pod finished, waiting for job status"
	case "Failed":
		// The Job is not terminal (no Failed condition), so more attempts
		// remain.
		exit, desc := lastTermination(pods, spec)
		st.ExitCode = exit
		st.Phase = executor.PhaseRetrying
		st.Message = "attempt failed"
		if desc != "" {
			st.Message += " (" + desc + ")"
		}
		st.Message += ", retrying"
	default:
		st.Phase = executor.PhasePending
	}
	return st
}

// lastTermination finds the most recent container termination across the
// pods (newest attempt first) and describes it: exit code, and OOM called out
// explicitly since "exit code 137" alone sends people the wrong way.
func lastTermination(pods []podDoc, spec executor.Spec) (*int, string) {
	byNewest := make([]podDoc, len(pods))
	copy(byNewest, pods)
	sort.Slice(byNewest, func(i, j int) bool {
		return byNewest[i].Metadata.CreationTimestamp.After(byNewest[j].Metadata.CreationTimestamp)
	})
	for i := range byNewest {
		for _, cs := range byNewest[i].Status.ContainerStatuses {
			term := cs.State.Terminated
			if term == nil {
				term = cs.LastState.Terminated
			}
			if term == nil {
				continue
			}
			exit := term.ExitCode
			if term.Reason == "OOMKilled" {
				desc := "out of memory (OOMKilled)"
				if spec.Resources.MemoryLimit != "" {
					desc += ", memory limit was " + spec.Resources.MemoryLimit
				}
				return &exit, desc
			}
			return &exit, fmt.Sprintf("last exit code %d", exit)
		}
	}
	return nil, ""
}

// newestPod returns the most recently created pod (the current attempt).
func newestPod(pods []podDoc) *podDoc {
	var newest *podDoc
	for i := range pods {
		if newest == nil || pods[i].Metadata.CreationTimestamp.After(newest.Metadata.CreationTimestamp) {
			newest = &pods[i]
		}
	}
	return newest
}

func waitingReason(pod *podDoc) *struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
} {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			return cs.State.Waiting
		}
	}
	return nil
}

func findPodCondition(pod *podDoc, typ string) *podCondition {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == typ {
			return &pod.Status.Conditions[i]
		}
	}
	return nil
}

func nodeSuffix(pod *podDoc) string {
	if pod.Spec.NodeName == "" {
		return ""
	}
	return " on " + pod.Spec.NodeName
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
