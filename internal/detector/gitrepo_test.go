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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
}

func TestAddGitignoreFromSample_CopiesTheSample(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore.sample"), "/vendor/\n")
	var lines []string

	addGitignoreFromSample(recordingConfig(dir, &lines), dir)

	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected a .gitignore: %v", err)
	}
	if string(got) != "/vendor/\n" {
		t.Errorf(".gitignore is %q, expected a copy of the sample", got)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Copied .gitignore.sample to .gitignore") {
		t.Errorf("log was %q, expected it to report the copy", lines)
	}
}

func TestAddGitignoreFromSample_LeavesAnExistingGitignoreAlone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore.sample"), "/vendor/\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "mine\n")
	var lines []string

	addGitignoreFromSample(recordingConfig(dir, &lines), dir)

	got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(got) != "mine\n" {
		t.Errorf(".gitignore is %q, expected the existing one to be kept", got)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "already has a .gitignore") {
		t.Errorf("log was %q, expected it to say the .gitignore was kept", lines)
	}
}

func TestAddGitignoreFromSample_SkipsWithoutASample(t *testing.T) {
	dir := t.TempDir()
	var lines []string

	addGitignoreFromSample(recordingConfig(dir, &lines), dir)

	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
		t.Error("a .gitignore was written without a sample to copy")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "no .gitignore.sample") {
		t.Errorf("log was %q, expected it to say there is no sample", lines)
	}
}

func TestAddGitignoreFromSample_ReportsAProblemInsteadOfFailingTheInstall(t *testing.T) {
	dir := t.TempDir()
	var lines []string

	addGitignoreFromSample(recordingConfig(dir, &lines), filepath.Join(dir, "missing"))

	if !strings.Contains(strings.Join(lines, "\n"), "Could not add .gitignore") {
		t.Errorf("log was %q, expected it to report the problem", lines)
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
