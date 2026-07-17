package deployer

import ghexec "github.com/example/ring-promoter/internal/executor/github"

// The GitHub API wire types moved to the execution backend along with the
// implementation; these aliases keep the long-standing tests in
// github_test.go building unchanged against the same fake server.
type (
	ghRun          = ghexec.Run
	ghRunsResponse = ghexec.RunsResponse
)
