package tui

import "github.com/charmbracelet/lipgloss"

func (m Model) categoriasLista() string {
	nomes := []string{"Todas", "Senhas", "Tokens", "Certificados", "SSH", "Auth/2FA", "Notas", "Bancos de dados"}
	for i := range nomes {
		if i == m.selectedCatIdx {
			nomes[i] = lipgloss.NewStyle().Bold(true).Foreground(m.cores().Destaque).Render("[" + nomes[i] + "]")
		} else {
			nomes[i] = lipgloss.NewStyle().Foreground(m.cores().Suave).Render(nomes[i])
		}
	}
	return wrapHelp(nomes, m.larguraAcesso()-4)
}

func nomeCategoriaLista(categoria string) string {
	switch categoria {
	case "password":
		return "Senha"
	case "token":
		return "Token"
	case "certificate":
		return "Certificado"
	case "ssh_key":
		return "SSH"
	case "auth":
		return "Auth/2FA"
	case "note":
		return "Nota"
	case "database":
		return "Banco dados"
	default:
		return categoria
	}
}
