// Package prereq checks, before anything is installed, the things an install
// would otherwise discover ten minutes in: a Docker daemon that is not
// running, no route to the package repository, a missing tool.
package prereq

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// Status is how a check turned out.
type Status int

const (
	// Pass means nothing to report.
	Pass Status = iota
	// Warn means the install can run, but something optional will be skipped.
	Warn
	// Fail means the install would not get far.
	Fail
)

// Result is one check's outcome, worded for the screen.
type Result struct {
	Name   string
	Status Status
	Detail string
}

// Check is one prerequisite and how to test it.
type Check struct {
	Name string
	Run  func() Result
}

// RepositoryURL is the package repository every install downloads from.
const RepositoryURL = "https://repo.mage-os.org/packages.json"

const (
	commandTimeout = 10 * time.Second
	networkTimeout = 5 * time.Second
)

// Default are the checks run before every install.
func Default() []Check {
	checks := []Check{
		{Name: "Docker", Run: dockerRunning},
		{Name: "Mage-OS repository", Run: repositoryReachable},
		{Name: "git", Run: optionalCommand("git", "the Git option will be skipped")},
	}
	if runtime.GOOS != "windows" {
		checks = append(checks, Check{Name: "sudo", Run: optionalCommand("sudo", "the environment cannot update /etc/hosts for you")})
	}
	return checks
}

// RunAll runs every check and returns the results in the same order.
func RunAll(checks []Check) []Result {
	results := make([]Result, 0, len(checks))
	for _, check := range checks {
		result := check.Run()
		result.Name = check.Name
		results = append(results, result)
	}
	return results
}

// AnyFailed reports whether the install should not start.
func AnyFailed(results []Result) bool {
	for _, result := range results {
		if result.Status == Fail {
			return true
		}
	}
	return false
}

// AnyWarned reports whether there is something the user should see first.
func AnyWarned(results []Result) bool {
	for _, result := range results {
		if result.Status == Warn {
			return true
		}
	}
	return false
}

// dockerRunning asks the daemon rather than checking for the binary: both
// DDEV and Warden fail instantly without it, and a stopped Docker Desktop is
// the most common reason an install dies at the first step.
func dockerRunning() Result {
	if _, err := exec.LookPath("docker"); err != nil {
		return Result{Status: Fail, Detail: "docker is not installed"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		return Result{Status: Fail, Detail: "the Docker daemon is not running"}
	}
	return Result{Status: Pass, Detail: "running"}
}

// repositoryReachable checks the route composer will need.
func repositoryReachable() Result {
	return reachable(RepositoryURL)
}

// reachable performs a HEAD request with a short timeout.
func reachable(url string) Result {
	client := &http.Client{Timeout: networkTimeout}
	response, err := client.Head(url)
	if err != nil {
		return Result{Status: Fail, Detail: fmt.Sprintf("cannot reach %s: %v", url, err)}
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return Result{Status: Fail, Detail: fmt.Sprintf("%s answered HTTP %d", url, response.StatusCode)}
	}
	return Result{Status: Pass, Detail: "reachable"}
}

// optionalCommand warns when a tool the install can do without is missing.
func optionalCommand(name, consequence string) func() Result {
	return func() Result {
		if _, err := exec.LookPath(name); err != nil {
			return Result{Status: Warn, Detail: name + " is not installed, " + consequence}
		}
		return Result{Status: Pass, Detail: "found"}
	}
}
