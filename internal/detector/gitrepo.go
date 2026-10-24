package detector

import (
	"fmt"
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

// containerProjectDir is where the project files are copied to inside the
// web container, in both DDEV and Warden.
const containerProjectDir = "/var/www/html"

// gitignoreFromSampleScript copies the .gitignore.sample Mage-OS ships with
// the project to .gitignore in the directory given as $1, and prints what it
// did. An existing .gitignore is never replaced.
const gitignoreFromSampleScript = `cd "$1" || exit 1
if [ -e .gitignore ] || [ -L .gitignore ]; then
	echo exists
elif [ -f .gitignore.sample ]; then
	cp .gitignore.sample .gitignore && echo copied
else
	echo missing
fi`

// addGitignoreFromSample gives the project a .gitignore from the
// .gitignore.sample Mage-OS ships with it. It runs through execPrefix (the
// container exec command) because once the project files are copied, the
// directory belongs to the container user and the host cannot write to it.
// Mage-OS versions without the sample get no .gitignore. Like git init, it
// cannot fail the install.
func addGitignoreFromSample(config *Config, projectDir string, execPrefix ...string) {
	args := append(append([]string{}, execPrefix...), "bash", "-c", gitignoreFromSampleScript, "bash", projectDir)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = config.Directory
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		logf(config, "⚠ Could not add .gitignore: %v", err)
		return
	}

	lines := strings.Fields(string(out))
	result := ""
	if len(lines) > 0 {
		result = lines[len(lines)-1]
	}
	switch result {
	case "copied":
		logf(config, "▸ Copied .gitignore.sample to .gitignore")
	case "exists":
		logf(config, "▸ The project already has a .gitignore, leaving it alone")
	case "missing":
		logf(config, "▸ This Mage-OS version has no .gitignore.sample, skipping .gitignore")
	default:
		logf(config, "⚠ Could not add .gitignore: unexpected output %q", strings.TrimSpace(string(out)))
	}
}
