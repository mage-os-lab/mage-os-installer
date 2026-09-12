package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// installLogName is the file every line of installer output is written to,
// in the project directory. It is created before anything else touches the
// directory, so it stays writable even where a container copy later changes
// the directory's owner.
const installLogName = ".mage-os-install.log"

// installLogPath is where the log for a project directory lives.
func installLogPath(dir string) string {
	return filepath.Join(dir, installLogName)
}

// installLog appends installer output to the log file. A nil installLog is
// valid and writes nothing, so callers need not care whether the file opened.
type installLog struct {
	file *os.File
}

// openInstallLog opens the log for appending and marks the start of this run,
// so retries in the same file can be told apart.
func openInstallLog(dir string) (*installLog, error) {
	file, err := os.OpenFile(installLogPath(dir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	log := &installLog{file: file}
	log.Write(fmt.Sprintf("=== mage-os-install run started %s ===", time.Now().Format(time.RFC3339)))
	return log, nil
}

// Write appends one line, without the colour codes the TUI shows.
func (l *installLog) Write(line string) {
	if l == nil {
		return
	}
	_, _ = l.file.WriteString(stripANSI(line) + "\n")
}

// Close finishes the file.
func (l *installLog) Close() {
	if l == nil {
		return
	}
	_ = l.file.Close()
}
