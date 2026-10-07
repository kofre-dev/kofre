package acesso

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRelogioRecuadoNaoCriaBloqueioIndefinido(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.enc")
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	data, err := json.Marshal(estado{Falhas: 3, Ate: agora.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path+".tentativas.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	if x, err := Novas(path).Iniciar(agora); err == nil {
		x.Fechar()
		t.Fatal("relógio recuado ignorou espera")
	}
	x, err := Novas(path).Iniciar(agora.Add(5 * time.Minute))
	if err != nil {
		t.Fatal("clamp não persistiu entre instâncias:", err)
	}
	x.Fechar()
}

func TestEsperaProgressivaPersistenteComExclusaoMutua(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.enc")
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for bloco, espera := range []int{5, 15, 30, 60, 120, 300, 300} {
		for tentativa := 0; tentativa < 3; tentativa++ {
			c := Novas(path) // Simula reinício entre cada tentativa.
			x, err := c.Iniciar(agora)
			if err != nil {
				t.Fatalf("bloco %d tentativa %d: %v", bloco, tentativa, err)
			}
			if outra, err := Novas(path).Iniciar(agora); err == nil {
				outra.Fechar()
				t.Fatal("segunda instância entrou enquanto a primeira autentica")
			}
			x.Fechar()
		}
		c := Novas(path)
		if x, err := c.Iniciar(agora); err == nil {
			x.Fechar()
			t.Fatal("limite de três tentativas não aplicado")
		}
		if got := c.Restante(agora); got != espera {
			t.Fatalf("espera %d; esperava %d", got, espera)
		}
		agora = agora.Add(time.Duration(espera) * time.Second)
	}
	x, err := Novas(path).Iniciar(agora)
	if err != nil {
		t.Fatal(err)
	}
	if err = x.Sucesso(); err != nil {
		t.Fatal(err)
	}
	x.Fechar()
	c := Novas(path)
	x, err = c.Iniciar(agora)
	if err != nil {
		t.Fatal("sucesso não zerou contador:", err)
	}
	x.Fechar()
	if c.estado.Falhas != 1 {
		t.Fatal("contador anterior permaneceu")
	}
}

func TestEstadoCorrompidoNaoLiberaTentativas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.enc")
	if err := os.WriteFile(path+".tentativas.json", []byte("arquivo inválido"), 0600); err != nil {
		t.Fatal(err)
	}
	if x, err := Novas(path).Iniciar(time.Now()); err == nil {
		x.Fechar()
		t.Fatal("estado inválido foi ignorado")
	}
}

func TestSucessoNaTerceiraTentativaNaoBloqueia(t *testing.T) {
	c := Novas("")
	agora := time.Now()
	for i := 0; i < 3; i++ {
		x, err := c.Iniciar(agora)
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			if err = x.Sucesso(); err != nil {
				t.Fatal(err)
			}
		}
		x.Fechar()
	}
	if c.Restante(agora) != 0 {
		t.Fatal("sucesso deixou espera ativa")
	}
}
