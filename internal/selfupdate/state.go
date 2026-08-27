package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/vindhyadatascience/vmup/internal/config"
)

// checkInterval is how long a check result is reused before another is made.
const checkInterval = 24 * time.Hour

// state caches the last update check.
//
// It lives in its own file rather than in settings.json because SaveSettings
// rewrites that whole file from a cached snapshot: a background writer racing
// the settings screen would silently discard whichever write landed first.
type state struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version,omitempty"`
}

func statePath() string {
	return filepath.Join(config.BaseDir(), "update-check.json")
}

func loadState() state {
	var s state
	data, err := os.ReadFile(statePath())
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return state{}
	}
	return s
}

// saveState records a check. Failures are ignored: a cache that cannot be
// written should slow vmup down, not break it.
func saveState(s state) {
	if err := os.MkdirAll(config.BaseDir(), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(config.BaseDir(), ".update-check-*")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return
	}
	if err := os.Rename(name, statePath()); err != nil {
		os.Remove(name)
	}
}

// fresh reports whether a cached result is recent enough to reuse.
//
// The timestamp is written even when a check FAILS, so a machine that is
// offline retries daily rather than on every single launch.
func (s state) fresh(now time.Time) bool {
	return !s.CheckedAt.IsZero() && now.Sub(s.CheckedAt) < checkInterval
}
