package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"kofre/pkg/vault"
)

const (
	campoTipoBanco    = 5
	campoHostBanco    = 6
	campoPortaBanco   = 7
	campoNomeBanco    = 8
	campoTLSBanco     = 9
	campoArquivoBanco = 10
)

var tiposBanco = []string{"PostgreSQL", "MySQL", "MariaDB", "SQL Server", "Firebird", "Oracle", "MongoDB", "Redis", "SQLite", "Outro"}
var modosTLSBanco = []string{"Validar certificado", "TLS sem validar certificado", "Desativado"}
var nomesCamposBanco = []string{"Tipo de banco", "Host", "Porta", "Banco de dados", "TLS", "Arquivo"}

func nomeCategoriaFormulario(cat vault.Category) string {
	if cat == vault.CategoryDatabase {
		return "Banco de dados"
	}
	return nomeCategoriaLista(string(cat))
}

func categoriaFormulario(value string) vault.Category {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "banco de dados" || value == "banco_de_dados" {
		return vault.CategoryDatabase
	}
	return vault.Category(value)
}

func (m Model) formularioBanco() bool {
	return len(m.formInputs) > campoArquivoBanco && categoriaFormulario(m.formInputs[1].Value()) == vault.CategoryDatabase
}

func (m Model) bancoSQLite() bool {
	return strings.EqualFold(strings.TrimSpace(m.formInputs[campoTipoBanco].Value()), "SQLite")
}

func portaPadraoBanco(tipo string) string {
	switch strings.ToLower(tipo) {
	case "postgresql":
		return "5432"
	case "mysql", "mariadb":
		return "3306"
	case "sql server":
		return "1433"
	case "firebird":
		return "3050"
	case "oracle":
		return "1521"
	case "mongodb":
		return "27017"
	case "redis":
		return "6379"
	}
	return ""
}

func (m *Model) initCamposBanco(entry vault.SecretEntry) {
	prompts := []string{"Tipo de banco: ", "Host: ", "Porta: ", "Banco de dados: ", "TLS: ", "Arquivo SQLite: "}
	placeholders := []string{"PostgreSQL", "db.exemplo.com ou 127.0.0.1", "1 a 65535 (opcional)", "Nome do banco / serviço (opcional)", "Validar certificado", "Caminho do arquivo .db"}
	for i, nome := range nomesCamposBanco {
		indice := campoTipoBanco + i
		m.formInputs[indice] = newInput(false)
		m.formInputs[indice].Prompt, m.formInputs[indice].Placeholder = prompts[i], placeholders[i]
		for _, field := range entry.Fields {
			if !field.Protected && strings.EqualFold(field.Name, nome) {
				m.formInputs[indice].SetValue(field.Value)
			}
		}
	}
	if m.formInputs[campoTipoBanco].Value() == "" {
		m.formInputs[campoTipoBanco].SetValue("PostgreSQL")
	}
	if m.formInputs[campoTLSBanco].Value() == "" {
		m.formInputs[campoTLSBanco].SetValue(modosTLSBanco[0])
	}
	if !m.isEditing && m.formInputs[campoPortaBanco].Value() == "" {
		m.formInputs[campoPortaBanco].SetValue(portaPadraoBanco(m.formInputs[campoTipoBanco].Value()))
	}
}

func (m Model) ordemFormulario() []int {
	if !m.formularioBanco() {
		return ordemCamposFormulario
	}
	if m.bancoSQLite() {
		return []int{1, 0, campoTipoBanco, campoArquivoBanco, 4}
	}
	return []int{1, 0, campoTipoBanco, campoHostBanco, campoPortaBanco, campoNomeBanco, 2, 3, campoTLSBanco, 4}
}

func (m Model) proximoCampoFormulario(atual, direcao int) int {
	ordem := m.ordemFormulario()
	for pos, indice := range ordem {
		if indice == atual {
			return ordem[(pos+direcao+len(ordem))%len(ordem)]
		}
	}
	return 1
}

func ciclarOpcao(valor string, opcoes []string, direcao int) string {
	for i, opcao := range opcoes {
		if strings.EqualFold(valor, opcao) {
			return opcoes[(i+direcao+len(opcoes))%len(opcoes)]
		}
	}
	return opcoes[0]
}

// Seletores usam setas laterais; Tab/Enter continuam percorrendo os campos.
func (m *Model) selecionarOpcaoFormulario(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyLeft && msg.Type != tea.KeyRight {
		return false
	}
	direcao := 1
	if msg.Type == tea.KeyLeft {
		direcao = -1
	}
	switch m.formFocusIndex {
	case 1:
		opcoes := make([]string, len(vault.AllCategories))
		for i, cat := range vault.AllCategories {
			opcoes[i] = string(cat)
		}
		m.formInputs[1].SetValue(ciclarOpcao(string(categoriaFormulario(m.formInputs[1].Value())), opcoes, direcao))
	case campoTipoBanco:
		if !m.formularioBanco() {
			return false
		}
		anterior := m.formInputs[campoTipoBanco].Value()
		novo := ciclarOpcao(anterior, tiposBanco, direcao)
		porta := m.formInputs[campoPortaBanco].Value()
		if porta == "" || porta == portaPadraoBanco(anterior) {
			m.formInputs[campoPortaBanco].SetValue(portaPadraoBanco(novo))
		}
		m.formInputs[campoTipoBanco].SetValue(novo)
	case campoTLSBanco:
		if !m.formularioBanco() {
			return false
		}
		m.formInputs[campoTLSBanco].SetValue(ciclarOpcao(m.formInputs[campoTLSBanco].Value(), modosTLSBanco, direcao))
	default:
		return false
	}
	return true
}

func (m Model) camposBanco() ([]vault.Field, error) {
	if !m.formularioBanco() {
		return nil, nil
	}
	tipo := strings.TrimSpace(m.formInputs[campoTipoBanco].Value())
	valido := false
	for _, opcao := range tiposBanco {
		if strings.EqualFold(tipo, opcao) {
			tipo, valido = opcao, true
			break
		}
	}
	if !valido {
		return nil, fmt.Errorf("selecione o tipo de banco com ←/→; use Outro para um tipo diferente")
	}
	indices := []int{campoTipoBanco, campoArquivoBanco}
	if m.bancoSQLite() {
		if strings.TrimSpace(m.formInputs[campoArquivoBanco].Value()) == "" {
			return nil, fmt.Errorf("informe o caminho do arquivo SQLite")
		}
	} else {
		if strings.TrimSpace(m.formInputs[campoHostBanco].Value()) == "" {
			return nil, fmt.Errorf("host do banco é obrigatório")
		}
		porta := strings.TrimSpace(m.formInputs[campoPortaBanco].Value())
		if porta != "" {
			n, err := strconv.Atoi(porta)
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("porta deve ser um número entre 1 e 65535")
			}
		}
		tls := strings.TrimSpace(m.formInputs[campoTLSBanco].Value())
		valido := false
		for _, opcao := range modosTLSBanco {
			if tls == opcao {
				valido = true
				break
			}
		}
		if !valido {
			return nil, fmt.Errorf("selecione a configuração TLS com ←/→")
		}
		indices = []int{campoTipoBanco, campoHostBanco, campoPortaBanco, campoNomeBanco, campoTLSBanco}
	}
	fields := make([]vault.Field, 0, len(indices))
	for _, indice := range indices {
		valor := strings.TrimSpace(m.formInputs[indice].Value())
		if indice == campoTipoBanco {
			valor = tipo
		}
		if valor != "" {
			fields = append(fields, vault.Field{Name: nomesCamposBanco[indice-campoTipoBanco], Value: valor})
		}
	}
	return fields, nil
}

func atualizarCamposBanco(entry *vault.SecretEntry, fields []vault.Field) {
	// Preserva outros campos/anexos da entrada e substitui apenas metadados da conexão.
	preservados := make([]vault.Field, 0, len(entry.Fields)+len(fields))
	for _, field := range entry.Fields {
		conexao := false
		for _, nome := range nomesCamposBanco {
			if strings.EqualFold(field.Name, nome) && !field.Protected {
				conexao = true
				break
			}
		}
		if !conexao {
			preservados = append(preservados, field)
		}
	}
	entry.Fields = append(preservados, fields...)
}
