// Package resume remembers how far an install got, so a run that stopped
// (a failure, a closed terminal, a laptop lid) can pick up where it was
// instead of starting with ddev config again.
package resume

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mage-os/mage-os-install/internal/detector"
)

// FileName is the state file in the project directory. Secrets never go in
// it: the admin password and the Hyvä token are asked for again on resume.
const FileName = ".mage-os-install.json"

// State is what a later run needs to continue an earlier one.
type State struct {
	Environment string            `json:"environment"`
	ProjectName string            `json:"project_name"`
	Fields      map[string]string `json:"fields"` // form label to value, secrets left out
	SampleData  bool              `json:"sample_data"`
	InitGit     bool              `json:"init_git"`
	Hyva        bool              `json:"hyva"`
	HyvaRepoURL string            `json:"hyva_repo_url,omitempty"`
	Steps       []string          `json:"steps"`     // the full step list of that run
	Completed   []string          `json:"completed"` // the steps that finished
}

// Path is where the state for a project directory lives.
func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

// Save writes the state, replacing any earlier one.
func Save(dir string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(dir), data, 0644)
}

// Load reads the state of an earlier run. The second value is false when
// there is none.
func Load(dir string) (State, bool, error) {
	data, err := os.ReadFile(Path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

// Clear forgets the earlier run, once it has finished or the user starts over.
func Clear(dir string) error {
	err := os.Remove(Path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// NextStep names the first step the earlier run did not finish.
func (s State) NextStep() string {
	done := completedSet(s.Completed)
	for _, step := range s.Steps {
		if !done[step] {
			return step
		}
	}
	return ""
}

// StartIndex is the step to resume from in a freshly built step list: the
// leading run of steps the earlier attempt finished. Steps are matched by
// name, so a changed option (say, sample data switched on) shortens the run
// rather than pointing at the wrong step.
func StartIndex(completed []string, steps []detector.Step) int {
	done := completedSet(completed)
	for i, step := range steps {
		if !done[step.Name] {
			return i
		}
	}
	return len(steps)
}

func completedSet(completed []string) map[string]bool {
	done := make(map[string]bool, len(completed))
	for _, name := range completed {
		done[name] = true
	}
	return done
}
