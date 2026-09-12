package detector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitGitRepository_CreatesARepositoryWithAGitignore(t *testing.T) {
	dir := t.TempDir()
	var lines []string

	initGitRepository(recordingConfig(dir, &lines))

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("expected a Git repository in %s: %v", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Errorf("expected a .gitignore in %s: %v", dir, err)
	}
}

func TestInitGitRepository_LeavesAnExistingGitignoreAlone(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, ".gitignore")
	var lines []string

	initGitRepository(recordingConfig(dir, &lines))

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("could not read .gitignore: %v", err)
	}
	if string(content) == gitignore {
		t.Error(".gitignore was overwritten, expected the existing one to be kept")
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
	if _, err := os.Stat(filepath.Join(project, ".gitignore")); err != nil {
		t.Errorf("expected the .gitignore to be written anyway: %v", err)
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

	if !strings.Contains(strings.Join(lines, "\n"), "Could not write .gitignore") {
		t.Errorf("log was %q, expected it to report the failed write", lines)
	}
}

func TestIsInsideGitWorkTree_IsFalseForAPlainDirectory(t *testing.T) {
	if isInsideGitWorkTree(t.TempDir()) {
		t.Error("a fresh directory should not be inside a work tree")
	}
}

func TestGitignore_TracksTheModuleList(t *testing.T) {
	if strings.Contains(gitignore, "/app/etc/config.php") {
		t.Error("app/etc/config.php should be tracked, so every checkout knows which modules to enable")
	}
}

func TestGitignore_IgnoresTheDevFolderAndTheDeploymentConfig(t *testing.T) {
	for _, want := range []string{"/dev", "/app/etc/env.php", "/vendor/*", "/generated/*", "/var/*"} {
		if !containsLine(gitignore, want) {
			t.Errorf("expected .gitignore to contain %q", want)
		}
	}
}

// containsLine reports whether content has want as a complete line.
func containsLine(content, want string) bool {
	for _, line := range strings.Split(content, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

func TestSteps_EndWithGitBeforeVerificationWhenRequested(t *testing.T) {
	for name, d := range map[string]Detector{"DDEV": &DdevDetector{}, "Warden": &WardenDetector{}} {
		d.PrepareSteps(&Config{ProjectName: "test-project", InitGit: true})
		steps := d.Steps()

		last := []string{steps[len(steps)-2].Name, steps[len(steps)-1].Name}
		if last[0] != "Initialize Git repository" || last[1] != "Verify installation" {
			t.Errorf("[%s] last steps are %q, expected the Git step just before verification", name, last)
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
