package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// namedEntries is how many entries the warning lists by name.
const namedEntries = 3

// existingContents describes what is already in dir, so the user can decide
// whether to install over it. It is empty when the directory does not exist
// yet or has nothing in it, which is the case the installer is built for.
func existingContents(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return ""
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)

	shown := names
	if len(shown) > namedEntries {
		shown = shown[:namedEntries]
	}
	return fmt.Sprintf("%s, including %s", countEntries(len(names)), strings.Join(shown, ", "))
}

// countEntries words the entry count.
func countEntries(count int) string {
	if count == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", count)
}
