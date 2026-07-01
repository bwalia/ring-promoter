package deployer

import (
	"context"
	"log/slog"
	"sync"
)

// LogDeployer is a no-op deployer that records what it would have done. It is
// the default for local development, where there is no cluster to talk to. It
// also remembers the last version deployed per target so it can satisfy the
// LiveVersioner interface.
type LogDeployer struct {
	log *slog.Logger

	mu   sync.Mutex
	live map[string]string // key: namespace/deployment/container
}

// NewLogDeployer returns a LogDeployer using the given logger.
func NewLogDeployer(log *slog.Logger) *LogDeployer {
	if log == nil {
		log = slog.Default()
	}
	return &LogDeployer{log: log, live: make(map[string]string)}
}

func liveKey(t Target) string { return t.Namespace + "/" + t.Deployment + "/" + t.Container }

// Deploy implements Deployer by logging the intended change.
func (d *LogDeployer) Deploy(_ context.Context, t Target, version string) error {
	d.mu.Lock()
	d.live[liveKey(t)] = version
	d.mu.Unlock()
	d.log.Info("log deployer: would deploy",
		"app", t.App, "ring", t.Ring, "namespace", t.Namespace,
		"deployment", t.Deployment, "container", t.Container,
		"image", t.Image+":"+version)
	return nil
}

// LiveVersion implements LiveVersioner from the in-memory record.
func (d *LogDeployer) LiveVersion(_ context.Context, t Target) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.live[liveKey(t)], nil
}
