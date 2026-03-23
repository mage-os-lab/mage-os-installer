package tui

import "github.com/charmbracelet/lipgloss"

var (
	orange  = lipgloss.Color("#F37B20")
	magenta = lipgloss.Color("#D94F8B")
	white   = lipgloss.Color("#FAFAFA")
	dim     = lipgloss.Color("#888888")
	green   = lipgloss.Color("#73D216")
	red     = lipgloss.Color("#EF2929")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(orange).
			MarginBottom(1)

	bannerStyle = lipgloss.NewStyle().
			Foreground(magenta).
			Bold(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(orange)

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(0).
				Foreground(orange).
				Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(dim)

	successStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(red).
			Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(orange).
			Padding(1, 2).
			MarginTop(1)

	logBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dim).
			Padding(0, 1).
			MarginTop(1).
			Width(80)

	highlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true)
)

const banner = `
 __  __                    ___  ____
|  \/  | __ _  __ _  ___ / _ \/ ___|
| |\/| |/ _` + "`" + ` |/ _` + "`" + ` |/ _ \ | | \___ \
| |  | | (_| | (_| |  __/ |_| |___) |
|_|  |_|\__,_|\__, |\___|\___/|____/
              |___/
`
