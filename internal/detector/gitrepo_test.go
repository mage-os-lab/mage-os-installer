package detector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitGitRepository_CreatesARepository(t *testing.T) {
	dir := t.TempDir()
	var lines []string

	initGitRepository(recordingConfig(dir, &lines))

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("expected a Git repository in %s: %v", dir, err)
	}
}

func TestInitGitRepository_LeavesTheGitignoreToMageOS(t *testing.T) {
	dir := t.TempDir()
	var lines []string

	initGitRepository(recordingConfig(dir, &lines))

	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
		t.Error("a .gitignore was written, expected Mage-OS to provide it")
	}
}

func TestInitGitRepository_DoesNotNestInsideAnExistingRepository(t *testing.T) {
	parent := t.TempDir()
	var parentLines []string
	initGitRepository(recordingConfig(parent, &parentLines))

	project := filepath.Join(parent, "shop")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatalf("could not create %s: %v", project, err)
	}
	var lines []string
	initGitRepository(recordingConfig(project, &lines))

	if _, err := os.Stat(filepath.Join(project, ".git")); err == nil {
		t.Error("a second repository was created inside an existing one")
	}
}

func TestInitGitRepository_ReportsAProblemInsteadOfFailingTheInstall(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("could not make %s read-only: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	var lines []string
	initGitRepository(recordingConfig(dir, &lines))

	if !strings.Contains(strings.Join(lines, "\n"), "git init failed") {
		t.Errorf("log was %q, expected it to report the failed git init", lines)
	}
}

func TestIsInsideGitWorkTree_IsFalseForAPlainDirectory(t *testing.T) {
	if isInsideGitWorkTree(t.TempDir()) {
		t.Error("a fresh directory should not be inside a work tree")
	}
}

func TestSteps_StartWithGitWhenRequested(t *testing.T) {
	for name, d := range map[string]Detector{"DDEV": &DdevDetector{}, "Warden": &WardenDetector{}} {
		d.PrepareSteps(&Config{ProjectName: "test-project", InitGit: true})

		// The repository has to exist before anything is copied into the
		// directory, or the installer can no longer write to it.
		if first := d.Steps()[0].Name; first != "Initialize Git repository" {
			t.Errorf("[%s] first step is %q, expected the Git step to come first", name, first)
		}
	}
}

func TestSteps_HaveNoGitStepWhenNotRequested(t *testing.T) {
	for name, d := range map[string]Detector{"DDEV": &DdevDetector{}, "Warden": &WardenDetector{}} {
		d.PrepareSteps(&Config{ProjectName: "test-project"})

		for _, step := range d.Steps() {
			if step.Name == "Initialize Git repository" {
				t.Errorf("[%s] did not ask for Git, but the step is there", name)
			}
		}
	}
}
