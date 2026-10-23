package k8sjob

import (
	"context"
	"time"

	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/progress"
)

// Stage ids of a Job's lifecycle, in order.
const (
	stageSchedule = "schedule"
	stagePull     = "pull"
	stageRun      = "run"
)

// Stages implements executor.StageReporter from the state the latest Status
// read observed: the current attempt's pod moves through scheduling, image
// pull and the deploy script itself. Before the first Status read there is
// nothing to describe, so it returns nil.
func (x *execution) Stages(context.Context) ([]progress.Stage, error) {
	x.mu.Lock()
	o := x.last
	x.mu.Unlock()
	if o == nil {
		return nil, nil
	}
	return jobStages(o.job, o.pods, o.phase), nil
}

// jobStages derives the three lifecycle stages of the newest pod. Each stage
// is done once the next one has evidence of starting; a terminal failure
// fails whichever stage was in progress and skips the rest.
func jobStages(job jobDoc, pods []podDoc, phase executor.Phase) []progress.Stage {
	sched := progress.Stage{ID: stageSchedule, Name: "Schedule pod", Status: progress.StageQueued}
	pull := progress.Stage{ID: stagePull, Name: "Pull image", Status: progress.StageQueued}
	run := progress.Stage{ID: stageRun, Name: "Run deploy script", Status: progress.StageQueued}

	if pod := newestPod(pods); pod != nil {
		created := pod.Metadata.CreationTimestamp
		sched.StartedAt = &created
		sched.Status = progress.StageRunning
		if c := findPodCondition(pod, "PodScheduled"); c != nil && c.Status == "True" {
			sched.Status = progress.StageSuccess
			sched.FinishedAt = c.LastTransitionTime
			pull.Status = progress.StageRunning
			pull.StartedAt = c.LastTransitionTime
		}
		for _, cs := range pod.Status.ContainerStatuses {
			var started, finished *time.Time
			switch {
			case cs.State.Running != nil:
				started = cs.State.Running.StartedAt
			case cs.State.Terminated != nil:
				started = cs.State.Terminated.StartedAt
				finished = cs.State.Terminated.FinishedAt
			default:
				continue
			}
			sched.Status = progress.StageSuccess
			pull.Status = progress.StageSuccess
			pull.FinishedAt = started
			run.Status = progress.StageRunning
			run.StartedAt = started
			run.FinishedAt = finished
		}
	}

	for _, c := range job.Status.Conditions {
		if c.Type == "Complete" && c.Status == "True" {
			for _, st := range []*progress.Stage{&sched, &pull, &run} {
				st.Status = progress.StageSuccess
			}
		}
	}
	if phase == executor.PhaseFailed || phase == executor.PhaseTimedOut || phase == executor.PhaseCancelled {
		end := progress.StageFailed
		if phase == executor.PhaseCancelled {
			end = progress.StageCancelled
		}
		failed := false
		for _, st := range []*progress.Stage{&sched, &pull, &run} {
			switch {
			case failed:
				st.Status = progress.StageSkipped
			case st.Status == progress.StageRunning:
				st.Status = end
				failed = true
			}
		}
		if !failed && run.Status == progress.StageSuccess {
			run.Status = end
		}
	}
	return []progress.Stage{sched, pull, run}
}
