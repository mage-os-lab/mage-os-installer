package resume

import (
	"os"
	"strings"
	"testing"

	"github.com/mage-os/mage-os-install/internal/detector"
)

func sample() State {
	return State{
		Environment: "DDEV",
		ProjectName: "shop",
		Fields:      map[string]string{"Admin user": "admin", "Admin email": "a@b.c"},
		InitGit:     true,
		Steps:       []string{"Configure", "Start", "Install Mage-OS", "Verify installation"},
		Completed:   []string{"Configure", "Start"},
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	if err := Save(dir, sample()); err != nil {
		t.Fatal(err)
	}
	got, found, err := Load(dir)

	if err != nil || !found {
		t.Fatalf("Load() = found %v, err %v", found, err)
	}
	if got.Environment != "DDEV" || got.Fields["Admin email"] != "a@b.c" || len(got.Completed) != 2 || !got.InitGit {
		t.Errorf("Load() = %+v", got)
	}
}

func TestLoad_ReportsNoStateWithoutAFile(t *testing.T) {
	_, found, err := Load(t.TempDir())

	if found || err != nil {
		t.Errorf("Load() = found %v, err %v, expected neither", found, err)
	}
}

func TestLoad_ReportsACorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Load(dir); err == nil {
		t.Error("expected an error for a corrupt state file")
	}
}

func TestClear_RemovesTheFileAndToleratesItsAbsence(t *testing.T) {
	dir := t.TempDir()
	_ = Save(dir, sample())

	if err := Clear(dir); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := Load(dir); found {
		t.Error("state should be gone after Clear")
	}
	if err := Clear(dir); err != nil {
		t.Errorf("Clear on a missing file should be fine, got %v", err)
	}
}

func TestSave_NeverWritesSecrets(t *testing.T) {
	dir := t.TempDir()
	state := sample()
	state.Fields["Admin user"] = "admin"

	_ = Save(dir, state)
	data, _ := os.ReadFile(Path(dir))

	for _, secret := range []string{"password", "token"} {
		if strings.Contains(strings.ToLower(string(data)), secret) {
			t.Errorf("state file should not carry a %s field, got:\n%s", secret, data)
		}
	}
}

func TestNextStep_IsTheFirstUnfinishedOne(t *testing.T) {
	if got := sample().NextStep(); got != "Install Mage-OS" {
		t.Errorf("NextStep() = %q", got)
	}
}

func TestStartIndex_SkipsTheLeadingFinishedSteps(t *testing.T) {
	steps := []detector.Step{{Name: "Configure"}, {Name: "Start"}, {Name: "Install Mage-OS"}, {Name: "Verify installation"}}

	if got := StartIndex([]string{"Configure", "Start"}, steps); got != 2 {
		t.Errorf("StartIndex() = %d, want 2", got)
	}
}

func TestStartIndex_StopsAtAStepTheEarlierRunNeverHad(t *testing.T) {
	// Sample data was switched on since; it sits before a finished step and
	// must run, so the resume point moves up to it.
	steps := []detector.Step{{Name: "Configure"}, {Name: "Install sample data"}, {Name: "Start"}}

	if got := StartIndex([]string{"Configure", "Start"}, steps); got != 1 {
		t.Errorf("StartIndex() = %d, want 1", got)
	}
}

func TestStartIndex_IsZeroWithNothingDone(t *testing.T) {
	if got := StartIndex(nil, []detector.Step{{Name: "Configure"}}); got != 0 {
		t.Errorf("StartIndex() = %d, want 0", got)
	}
}
