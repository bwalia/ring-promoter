package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/example/ring-promoter/internal/progress"
)

// runJobs is the subset of GET /actions/runs/{id}/jobs the stage view uses.
type runJobs struct {
	Jobs []runJob `json:"jobs"`
}

type runJob struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	HTMLURL     string     `json:"html_url"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Steps       []runStep  `json:"steps"`
}

type runStep struct {
	Number      int        `json:"number"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// Stages implements executor.StageReporter: each job of the workflow run (its
// latest attempt) is one stage, with the job's steps as the stage's steps.
func (x *execution) Stages(ctx context.Context) ([]progress.Stage, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/jobs?filter=latest&per_page=100",
		x.e.cfg.APIBaseURL, x.e.cfg.Owner, x.e.cfg.Repo, x.runID)
	resp, err := x.e.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	var body runJobs
	if derr := decode(resp, &body); derr != nil {
		return nil, derr
	}
	stages := make([]progress.Stage, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		st := progress.Stage{
			ID:         strconv.FormatInt(j.ID, 10),
			Name:       j.Name,
			Status:     stageStatus(j.Status, j.Conclusion),
			URL:        j.HTMLURL,
			StartedAt:  j.StartedAt,
			FinishedAt: j.CompletedAt,
		}
		for _, s := range j.Steps {
			st.Steps = append(st.Steps, progress.StageStep{
				Number:     s.Number,
				Name:       s.Name,
				Status:     stageStatus(s.Status, s.Conclusion),
				StartedAt:  s.StartedAt,
				FinishedAt: s.CompletedAt,
			})
		}
		stages = append(stages, st)
	}
	return stages, nil
}

// stageStatus maps a GitHub job/step status + conclusion onto a stage status.
func stageStatus(status, conclusion string) string {
	switch status {
	case "queued", "waiting", "pending", "requested":
		return progress.StageQueued
	case "in_progress":
		return progress.StageRunning
	case "completed":
		switch conclusion {
		case "success":
			return progress.StageSuccess
		case "skipped", "neutral":
			return progress.StageSkipped
		case "cancelled":
			return progress.StageCancelled
		}
		return progress.StageFailed
	}
	return progress.StageQueued
}
