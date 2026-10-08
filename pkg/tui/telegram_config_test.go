package tui

import (
	"errors"
	"testing"

	"kofre/pkg/config"
)

func TestTelegramFalhaPermaneceVisivelSemFecharCofre(t *testing.T) {
	m := fixtureModel(t)
	m.state = ViewList
	atualizado, cmd := m.Update(telegramEnvelopeSavedMsg{err: errors.New("HTTP 403")})
	atual := atualizado.(Model)
	if atual.err == nil || atual.state != ViewList || atual.notification != "" || cmd != nil {
		t.Fatal("falha foi ocultada ou alterou o acesso local")
	}
}

func TestTelegramSomenteQuandoProConfigurado(t *testing.T) {
	cfg := &config.AppConfig{CloudEnabled: true, KofreToken: "kfr_conta_teste", PlanoCloud: "free"}
	if deveConfigurarTelegram(cfg) {
		t.Fatal("Free tentou configurar Telegram")
	}
	cfg.TelegramAuth = true
	if deveConfigurarTelegram(cfg) {
		t.Fatal("preferência local concedeu recurso Pro ao Free")
	}
	cfg.PlanoCloud = "pro"
	if !deveConfigurarTelegram(cfg) {
		t.Fatal("Pro configurado não habilitou tentativa")
	}
	cfg.TelegramAuth = false
	if deveConfigurarTelegram(cfg) {
		t.Fatal("Telegram foi configurado sem opção do usuário")
	}
}
