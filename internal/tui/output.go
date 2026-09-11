package tui

import (
	"regexp"
	"strings"
)

// ansiEscape matches the colour codes DDEV, Composer and Symfony write.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// commandSynopsis matches the usage line Symfony prints after a command fails,
// e.g. "setup:install [--backend-frontname BACKEND-FRONTNAME] [...]". It runs
// for thousands of characters and pushes the real error off the screen.
var commandSynopsis = regexp.MustCompile(`^\S+ \[-`)

// commandEcho matches the line DDEV prints when a command exits non-zero. It
// repeats the entire command, admin password included, so it is dropped
// instead of being shown.
var commandEcho = regexp.MustCompile("^Failed to execute command `")

// maxErrorLines is how many lines of output the failure screen shows.
const maxErrorLines = 12

// installErrorSummary reduces raw installer output to the lines that explain
// the failure: colour codes stripped, blank lines and the command synopsis
// dropped, and only the last maxErrorLines kept.
func installErrorSummary(lines []string) []string {
	var kept []string
	for _, line := range lines {
		line = strings.TrimRight(stripANSI(line), " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if commandSynopsis.MatchString(line) || commandEcho.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > maxErrorLines {
		kept = kept[len(kept)-maxErrorLines:]
	}
	return kept
}

// stripANSI removes escape sequences so output stays readable and keeps its
// real width inside a bordered box.
func stripANSI(line string) string {
	return ansiEscape.ReplaceAllString(line, "")
}

// truncateLine shortens line to width characters, marking the cut with an
// ellipsis, so a long line wraps the box instead of breaking it.
func truncateLine(line string, width int) string {
	if width < 1 {
		return line
	}
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
