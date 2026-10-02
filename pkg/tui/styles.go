package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Paleta de cores
	colorBgDark   = lipgloss.Color("#121217")
	colorBorder   = lipgloss.Color("#2A2A38")
	colorAccent   = lipgloss.Color("#7D56F4")
	colorSuccess  = lipgloss.Color("#00E676")
	colorWarning  = lipgloss.Color("#FFD600")
	colorDanger   = lipgloss.Color("#FF1744")
	colorMuted    = lipgloss.Color("#666680")
	colorText     = lipgloss.Color("#E0E0F0")
	colorSelected = lipgloss.Color("#1E1B4B")

	// Estilos basicos
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Padding(0, 1)

	headerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	statusBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#2563EB")).
				Padding(0, 1).
				Bold(true)

	searchStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorBorder).
			Foreground(colorText)

	selectedItemStyle = lipgloss.NewStyle().
				Background(colorSelected).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				PaddingLeft(1)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(colorText).
			PaddingLeft(1)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	dangerStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	// Badges de categoria
	badgePassword = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#10B981")).
			Padding(0, 1).
			Bold(true)

	badgeToken = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#8B5CF6")).
			Padding(0, 1).
			Bold(true)

	badgeCert = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#F59E0B")).
			Padding(0, 1).
			Bold(true)

	badgeSSH = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#3B82F6")).
			Padding(0, 1).
			Bold(true)

	badgeAuth = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#EC4899")).
			Padding(0, 1).
			Bold(true)

	badgeNote = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#6B7280")).
			Padding(0, 1).
			Bold(true)
)

func renderCategoryBadge(cat string) string {
	switch cat {
	case "password":
		return badgePassword.Render("SENHA")
	case "token":
		return badgeToken.Render("TOKEN")
	case "certificate":
		return badgeCert.Render("CERT")
	case "ssh_key":
		return badgeSSH.Render("SSH")
	case "auth":
		return badgeAuth.Render("AUTH/2FA")
	case "note":
		return badgeNote.Render("NOTA")
	default:
		return badgeNote.Render(cat)
	}
}
