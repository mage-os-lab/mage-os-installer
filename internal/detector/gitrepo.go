package detector

import (
	"os/exec"
	"strings"
)

// initGitRepository turns the project directory into a Git repository. What
// to ignore is left to the .gitignore Mage-OS ships with the project. It
// cannot fail the install: a store without a repository is still worth having.
// Whatever goes wrong is reported instead.
func initGitRepository(config *Config) {
	if _, err := exec.LookPath("git"); err != nil {
		logf(config, "⚠ Git is not installed, skipping git init")
		return
	}
	if isInsideGitWorkTree(config.Directory) {
		logf(config, "▸ Already inside a Git repository, skipping git init")
		return
	}

	logf(config, "▸ git init")
	if err := runInDir(config.Directory, config.Log, "git", "init"); err != nil {
		logf(config, "⚠ git init failed: %v", err)
	}
}

// isInsideGitWorkTree reports whether dir already belongs to a repository, so
// git init does not nest a second one inside it.
func isInsideGitWorkTree(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}
