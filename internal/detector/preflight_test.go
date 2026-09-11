package detector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeArtifacts creates the given files, parent directories included.
func writeArtifacts(t *testing.T, dir string, artifacts ...string) {
	t.Helper()
	for _, artifact := range artifacts {
		path := filepath.Join(dir, artifact)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("could not create %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("<?php return [];"), 0644); err != nil {
			t.Fatalf("could not write %s: %v", path, err)
		}
	}
}

// recordingConfig is a Config that keeps the lines written to its log.
func recordingConfig(dir string, lines *[]string) *Config {
	return &Config{
		Directory: dir,
		Log:       func(line string) { *lines = append(*lines, line) },
	}
}

func TestFindStaleInstallArtifacts_FindsNothingInAFreshProject(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, filepath.Join("app", "etc", "di.xml"))

	if found := findStaleInstallArtifacts(dir); found != nil {
		t.Errorf("got %q, expected nothing to clean up", found)
	}
}

func TestFindStaleInstallArtifacts_IgnoresConfigPhpWithoutADeploymentConfig(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, filepath.Join("app", "etc", "config.php"))

	if found := findStaleInstallArtifacts(dir); found != nil {
		t.Errorf("got %q, expected a module list alone to be left alone", found)
	}
}

func TestFindStaleInstallArtifacts_FindsEveryLeftoverOfAFailedAttempt(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, staleInstallArtifacts...)

	found := findStaleInstallArtifacts(dir)

	if len(found) != len(staleInstallArtifacts) {
		t.Fatalf("got %q, expected %q", found, staleInstallArtifacts)
	}
}

func TestFindStaleInstallArtifacts_FindsADeploymentConfigOnItsOwn(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, deploymentConfig)

	found := findStaleInstallArtifacts(dir)

	if len(found) != 1 || found[0] != deploymentConfig {
		t.Errorf("got %q, expected %q", found, deploymentConfig)
	}
}

func TestClearStaleInstallArtifacts_RemovesThemBeforeTheInstall(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, staleInstallArtifacts...)
	var lines []string

	if err := clearStaleInstallArtifacts(recordingConfig(dir, &lines)); err != nil {
		t.Fatalf("clearStaleInstallArtifacts() = %v, expected no error", err)
	}

	for _, artifact := range staleInstallArtifacts {
		if _, err := os.Stat(filepath.Join(dir, artifact)); err == nil {
			t.Errorf("%s still exists", artifact)
		}
	}
}

func TestClearStaleInstallArtifacts_SaysWhatItRemoved(t *testing.T) {
	dir := t.TempDir()
	writeArtifacts(t, dir, deploymentConfig)
	var lines []string

	if err := clearStaleInstallArtifacts(recordingConfig(dir, &lines)); err != nil {
		t.Fatalf("clearStaleInstallArtifacts() = %v, expected no error", err)
	}

	if !strings.Contains(strings.Join(lines, "\n"), deploymentConfig) {
		t.Errorf("log was %q, expected it to name the file that was removed", lines)
	}
}

func TestClearStaleInstallArtifacts_StaysQuietInAFreshProject(t *testing.T) {
	dir := t.TempDir()
	var lines []string

	if err := clearStaleInstallArtifacts(recordingConfig(dir, &lines)); err != nil {
		t.Fatalf("clearStaleInstallArtifacts() = %v, expected no error", err)
	}

	if len(lines) != 0 {
		t.Errorf("log was %q, expected nothing to be reported", lines)
	}
}
