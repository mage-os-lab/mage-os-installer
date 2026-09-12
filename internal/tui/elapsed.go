package tui

import (
	"fmt"
	"time"
)

// now is the clock the install screen reads. Tests replace it, so a step can
// be a few minutes old without anyone waiting.
var now = time.Now

// formatElapsed renders a duration the way a person reads a stopwatch:
// seconds until a minute, then minutes and seconds, then hours and minutes.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// elapsed is how long a step has been running, or took if it has finished.
func (s installStep) elapsed() time.Duration {
	if s.startedAt.IsZero() {
		return 0
	}
	if s.finishedAt.IsZero() {
		return now().Sub(s.startedAt)
	}
	return s.finishedAt.Sub(s.startedAt)
}
