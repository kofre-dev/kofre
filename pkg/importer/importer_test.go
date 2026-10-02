package importer

import (
	"os"
	"testing"
)

func TestParseMessyTxt(t *testing.T) {
	content := `=== BANCO PRODUCAO ===
url: postgresql://192.168.1.50:5432/financeiro
user: postgres_admin
senha = MinhaSenhaSuperSecreta@999
porta: 5432

OpenAI Token Pessoal
sk-proj-948192847192847192847192847192847192

[Anotacoes do Servidor Antigo]
Lembrar de renovar o dominio dia 15
O backup roda todo dia as 03h da manha
contato suporte: suporte@provedor.com`

	tmpFile, err := os.CreateTemp("", "messy-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString(content)
	tmpFile.Close()

	entries, err := ParseMessyTxt(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseMessyTxt falhou: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("esperava 3 blocos de credenciais, obteve %d", len(entries))
	}

	// Bloco 1: Banco Producao
	banco := entries[0]
	if banco.Title != "BANCO PRODUCAO" {
		t.Errorf("titulo esperado 'BANCO PRODUCAO', obteve '%s'", banco.Title)
	}
	hasUser := false
	hasPass := false
	for _, f := range banco.Fields {
		if f.Name == "user" && f.Value == "postgres_admin" {
			hasUser = true
		}
		if f.Name == "senha" && f.Protected && f.Value == "MinhaSenhaSuperSecreta@999" {
			hasPass = true
		}
	}
	if !hasUser || !hasPass {
		t.Errorf("campos de usuario ou senha nao foram extraidos corretamente")
	}

	// Bloco 2: OpenAI Token
	openai := entries[1]
	hasOpenAIToken := false
	for _, f := range openai.Fields {
		if f.Name == "OPENAI_API_KEY" && f.Protected {
			hasOpenAIToken = true
		}
	}
	if !hasOpenAIToken {
		t.Errorf("token da OpenAI nao foi identificado pelo regex")
	}

	// Bloco 3: Anotacoes
	anotacao := entries[2]
	if anotacao.Title != "Anotacoes do Servidor Antigo" {
		t.Errorf("titulo de anotacao incorreto: %s", anotacao.Title)
	}
	if len(anotacao.Notes) == 0 {
		t.Errorf("anotacoes deveriam ter sido preservadas como notas")
	}
}

func TestParseCSV(t *testing.T) {
	csvContent := `Title,Category,Username,Password,URL,Notes
GitHub Deploy,token,developer_deploy,ghp_superSecretToken123456789012,https://github.com,Deploy token
AWS Root,password,root_admin,SuperSecretPass#999,https://aws.amazon.com,Conta principal
Banco Inter,password,empresa@gmail.com,SenhaInter123,,PIX cadastrado`

	tmpFile, err := os.CreateTemp("", "test-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString(csvContent)
	tmpFile.Close()

	entries, err := ParseTarget(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseTarget CSV falhou: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("esperava 3 entradas no CSV, obteve %d", len(entries))
	}

	// 1. GitHub Deploy
	e1 := entries[0]
	if e1.Title != "GitHub Deploy" || e1.Category != "token" {
		t.Errorf("item 1 incorreto: %+v", e1)
	}
	foundPass := false
	for _, f := range e1.Fields {
		if f.Name == "Senha" && f.Protected && f.Value == "ghp_superSecretToken123456789012" {
			foundPass = true
		}
	}
	if !foundPass {
		t.Errorf("campo Senha protegida não encontrado no item 1")
	}

	// 3. Banco Inter (sem URL)
	e3 := entries[2]
	if e3.Title != "Banco Inter" || e3.Notes != "PIX cadastrado" {
		t.Errorf("item 3 incorreto: %+v", e3)
	}
}

func TestParseJSON(t *testing.T) {
	jsonContent := `[
		{
			"title": "Stripe API Key",
			"category": "token",
			"username": "acct_prod",
			"password": "sk_live_948192849182948192849182",
			"url": "https://dashboard.stripe.com",
			"notes": "Chave de produção"
		},
		{
			"title": "Email Corporativo",
			"category": "password",
			"username": "contato@empresa.com",
			"password": "MinhaSenha@Email123"
		}
	]`

	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString(jsonContent)
	tmpFile.Close()

	entries, err := ParseTarget(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseTarget JSON falhou: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("esperava 2 entradas no JSON, obteve %d", len(entries))
	}

	e1 := entries[0]
	if e1.Title != "Stripe API Key" || e1.Category != "token" || e1.Notes != "Chave de produção" {
		t.Errorf("item 1 do JSON incorreto: %+v", e1)
	}
}

func TestParseChromeAndEdgeCSV(t *testing.T) {
	// Formato exato exportado pelo Google Chrome e Microsoft Edge:
	// name,url,username,password,note
	chromeCSV := `name,url,username,password,note
Google Accounts,https://accounts.google.com/signin,usuario@gmail.com,SenhaGoogle!999,Conta pessoal
Netflix,https://www.netflix.com/login,familia@netflix.com,SenhaNetflix@2026,
,https://app.slack.com/client/T123/C456,colaborador@empresa.com,SlackPass#777,Nota do Workspace Slack`

	tmpFile, err := os.CreateTemp("", "chrome-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString(chromeCSV)
	tmpFile.Close()

	entries, err := ParseTarget(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseTarget Chrome CSV falhou: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("esperava 3 entradas do Chrome CSV, obteve %d", len(entries))
	}

	// 1. Google Accounts
	g := entries[0]
	if g.Title != "Google Accounts" || g.Category != "password" || g.Notes != "Conta pessoal" {
		t.Errorf("item 1 (Google) incorreto: %+v", g)
	}
	foundGooglePass := false
	for _, f := range g.Fields {
		if f.Name == "Senha" && f.Protected && f.Value == "SenhaGoogle!999" {
			foundGooglePass = true
		}
	}
	if !foundGooglePass {
		t.Errorf("senha protegida do Google não encontrada")
	}

	// 2. Netflix (sem notas)
	n := entries[1]
	if n.Title != "Netflix" || n.Notes != "" {
		t.Errorf("item 2 (Netflix) incorreto: %+v", n)
	}

	// 3. Slack (name em branco, deve derivar o título do host da URL 'app.slack.com')
	s := entries[2]
	if s.Title != "app.slack.com" {
		t.Errorf("esperava que titulo em branco derivasse o host 'app.slack.com', obteve '%s'", s.Title)
	}
	if s.Notes != "Nota do Workspace Slack" {
		t.Errorf("nota do item 3 incorreta: %s", s.Notes)
	}
	foundSlackPass := false
	for _, f := range s.Fields {
		if f.Name == "Senha" && f.Protected && f.Value == "SlackPass#777" {
			foundSlackPass = true
		}
	}
	if !foundSlackPass {
		t.Errorf("senha protegida do Slack não encontrada")
	}
}


