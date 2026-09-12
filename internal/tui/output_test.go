package tui

import (
	"strings"
	"testing"
)

// failedSetupInstall is the output DDEV and Symfony produce when
// bin/magento setup:install rejects a parameter.
var failedSetupInstall = []string{
	"Your password must include both numeric and alphabetic characters.",
	"",
	"In InstallCommand.php line 291:",
	"                               ",
	"  Parameter validation failed  ",
	"                               ",
	"",
	"setup:install [--backend-frontname BACKEND-FRONTNAME] [--enable-debug-logging ENABLE-DEBUG-LOGGING] [--key KEY]",
	"",
	"\x1b[31mFailed to execute command `bin/magento setup:install --admin-password=Sup3rSecret!`: exit status 1\x1b[0m",
}

func TestInstallErrorSummary_KeepsTheMessageThatExplainsTheFailure(t *testing.T) {
	summary := installErrorSummary(failedSetupInstall)

	want := []string{
		"Your password must include both numeric and alphabetic characters.",
		"In InstallCommand.php line 291:",
		"  Parameter validation failed",
	}
	if len(summary) != len(want) {
		t.Fatalf("got %d lines %q, expected %d", len(summary), summary, len(want))
	}
	for i, line := range want {
		if summary[i] != line {
			t.Errorf("line %d = %q, expected %q", i, summary[i], line)
		}
	}
}

func TestInstallErrorSummary_DropsTheCommandSynopsis(t *testing.T) {
	for _, line := range installErrorSummary(failedSetupInstall) {
		if strings.Contains(line, "--backend-frontname") {
			t.Errorf("summary should not contain the command synopsis, got %q", line)
		}
	}
}

func TestInstallErrorSummary_DropsTheEchoedCommandWithThePassword(t *testing.T) {
	for _, line := range installErrorSummary(failedSetupInstall) {
		if strings.Contains(line, "Sup3rSecret") {
			t.Errorf("summary should not repeat the admin password, got %q", line)
		}
	}
}

func TestInstallErrorSummary_StripsColourCodes(t *testing.T) {
	summary := installErrorSummary([]string{"\x1b[31mComposer could not resolve dependencies\x1b[0m"})

	if len(summary) != 1 || summary[0] != "Composer could not resolve dependencies" {
		t.Errorf("got %q, expected the line without escape codes", summary)
	}
}

func TestInstallErrorSummary_KeepsOnlyTheLastLines(t *testing.T) {
	var lines []string
	for i := 0; i < maxErrorLines*2; i++ {
		lines = append(lines, string(rune('a'+i)))
	}

	summary := installErrorSummary(lines)

	if len(summary) != maxErrorLines {
		t.Fatalf("got %d lines, expected %d", len(summary), maxErrorLines)
	}
	if summary[len(summary)-1] != lines[len(lines)-1] {
		t.Errorf("last summary line = %q, expected %q", summary[len(summary)-1], lines[len(lines)-1])
	}
}

func TestInstallErrorSummary_ReturnsNothingForEmptyOutput(t *testing.T) {
	if summary := installErrorSummary([]string{"", "   "}); len(summary) != 0 {
		t.Errorf("got %q, expected no lines", summary)
	}
}

func TestWrapLine_LeavesShortLinesAlone(t *testing.T) {
	if got := wrapLine("short", 40); got != "short" {
		t.Errorf("got %q, expected %q", got, "short")
	}
}

func TestWrapLine_KeepsEveryWordOfALongLine(t *testing.T) {
	message := "step \"Initialize Git repository\" failed: could not write .gitignore: permission denied"

	got := wrapLine(message, 30)

	if !strings.Contains(got, "\n") {
		t.Error("expected the line to be wrapped over several lines")
	}
	for _, word := range strings.Fields(message) {
		if !strings.Contains(got, word) {
			t.Errorf("wrapped line lost %q:\n%s", word, got)
		}
	}
}

func TestWrapLine_LeavesTheLineAloneWhenTheWidthIsUnknown(t *testing.T) {
	message := strings.Repeat("a", 200)

	if got := wrapLine(message, 0); got != message {
		t.Errorf("got %q, expected the line unchanged", got)
	}
}
