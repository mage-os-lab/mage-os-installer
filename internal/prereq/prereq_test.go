package prereq

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunAll_KeepsOrderAndNamesResults(t *testing.T) {
	checks := []Check{
		{Name: "first", Run: func() Result { return Result{Status: Pass} }},
		{Name: "second", Run: func() Result { return Result{Status: Fail, Detail: "nope"} }},
	}

	results := RunAll(checks)

	if len(results) != 2 || results[0].Name != "first" || results[1].Name != "second" {
		t.Errorf("RunAll() = %+v, expected the checks' names in order", results)
	}
	if results[1].Status != Fail || results[1].Detail != "nope" {
		t.Errorf("RunAll() lost the second result: %+v", results[1])
	}
}

func TestAnyFailed_IgnoresWarnings(t *testing.T) {
	results := []Result{{Status: Pass}, {Status: Warn}}

	if AnyFailed(results) {
		t.Error("a warning should not count as a failure")
	}
	if !AnyWarned(results) {
		t.Error("a warning should be reported as such")
	}
	if !AnyFailed(append(results, Result{Status: Fail})) {
		t.Error("a failure should be reported")
	}
}

func TestReachable_PassesForAServerThatAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	if result := reachable(server.URL); result.Status != Pass {
		t.Errorf("reachable() = %+v, expected Pass", result)
	}
}

func TestReachable_FailsForAServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result := reachable(server.URL)

	if result.Status != Fail || !strings.Contains(result.Detail, "HTTP 500") {
		t.Errorf("reachable() = %+v, expected a Fail naming the status", result)
	}
}

func TestReachable_FailsWhenNothingListens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	result := reachable(url)

	if result.Status != Fail || !strings.Contains(result.Detail, "cannot reach") {
		t.Errorf("reachable() = %+v, expected a Fail saying it cannot reach the host", result)
	}
}

func TestOptionalCommand_WarnsWhenMissing(t *testing.T) {
	result := optionalCommand("definitely-not-a-command-on-this-machine", "something is skipped")()

	if result.Status != Warn || !strings.Contains(result.Detail, "something is skipped") {
		t.Errorf("optionalCommand() = %+v, expected a Warn with the consequence", result)
	}
}

func TestOptionalCommand_PassesWhenPresent(t *testing.T) {
	if result := optionalCommand("go", "")(); result.Status != Pass {
		t.Errorf("optionalCommand(go) = %+v, expected Pass", result)
	}
}

func TestDefault_ChecksTheThingsAnInstallDependsOn(t *testing.T) {
	var names []string
	for _, check := range Default() {
		names = append(names, check.Name)
	}
	joined := strings.Join(names, ",")

	for _, want := range []string{"Docker", "Mage-OS repository", "git"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Default() should include a %q check, got %q", want, joined)
		}
	}
}
