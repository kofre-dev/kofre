// Package acesso limita tentativas pelos clientes cooperantes. O estado local
// não impede ataques sobre cópias do cofre nem adulteração pelo dono do sistema.
package acesso

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"kofre/internal/arquivo"
	mycrypto "kofre/pkg/crypto"
)

type estado struct {
	Falhas int       `json:"tentativas_sem_sucesso"`
	Ate    time.Time `json:"aguardar_ate,omitempty"`
}

type Tentativas struct {
	mu      sync.Mutex
	caminho string
	estado  estado
}

func Novas(caminhoCofre string) *Tentativas {
	c := &Tentativas{}
	if caminhoCofre != "" {
		c.caminho = caminhoCofre + ".tentativas.json"
	}
	return c
}

// Tentativa mantém a exclusão mútua até o fim da derivação/autenticação.
type Tentativa struct {
	controle *Tentativas
	lock     *os.File
}

func (c *Tentativas) Iniciar(agora time.Time) (*Tentativa, error) {
	if !c.mu.TryLock() {
		return nil, errors.New("outra tentativa está em andamento")
	}
	t := &Tentativa{controle: c}
	falhou := true
	defer func() {
		if falhou {
			t.Fechar()
		}
	}()
	if c.caminho != "" {
		f, err := os.OpenFile(c.caminho+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("controle de tentativas: %w", err)
		}
		t.lock = f
		if err = bloquearArquivo(f); err != nil {
			return nil, errors.New("outra instância está abrindo este cofre; aguarde")
		}
		if err = mycrypto.RestrictFilePermissions(c.caminho + ".lock"); err != nil {
			return nil, err
		}
		if err = c.carregar(); err != nil {
			return nil, err
		}
	}
	// Limita esperas acidentais por relógio recuado/estado danificado a cinco minutos.
	if c.estado.Ate.After(agora.Add(5 * time.Minute)) {
		c.estado.Ate = agora.Add(5 * time.Minute)
		if err := c.salvar(); err != nil {
			return nil, err
		}
	}
	if restante := c.estado.Ate.Sub(agora); restante > 0 {
		return nil, fmt.Errorf("aguarde %d segundos antes de tentar novamente", segundos(restante))
	}
	c.estado.Falhas++
	if c.estado.Falhas%3 == 0 {
		esperas := []time.Duration{5, 15, 30, 60, 120, 300}
		indice := min(c.estado.Falhas/3-1, len(esperas)-1)
		c.estado.Ate = agora.Add(esperas[indice] * time.Second)
		// Evita overflow sem perder a cadência de três tentativas.
		if c.estado.Falhas >= 18 {
			c.estado.Falhas = 18
		}
	}
	// Reserva a tentativa antes da KDF: encerrar o processo não desfaz o contador.
	if err := c.salvar(); err != nil {
		return nil, err
	}
	falhou = false
	return t, nil
}

func segundos(d time.Duration) int { return int((d + time.Second - 1) / time.Second) }

// Restante usa a última leitura; Iniciar sempre relê sob lock entre processos.
func (c *Tentativas) Restante(agora time.Time) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return max(0, segundos(c.estado.Ate.Sub(agora)))
}

func (t *Tentativa) Sucesso() error {
	if t == nil || t.controle == nil {
		return errors.New("tentativa encerrada")
	}
	t.controle.estado = estado{}
	return t.controle.salvar()
}

func (t *Tentativa) Fechar() {
	if t == nil || t.controle == nil {
		return
	}
	if t.lock != nil {
		_ = t.lock.Close()
	}
	t.controle.mu.Unlock()
	t.controle = nil
}

func (c *Tentativas) carregar() error {
	f, err := os.Open(c.caminho)
	if errors.Is(err, os.ErrNotExist) {
		c.estado = estado{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("não foi possível ler o controle de tentativas: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 2049))
	if err != nil {
		return err
	}
	var e estado
	if len(b) > 2048 || json.Unmarshal(b, &e) != nil || e.Falhas < 0 || e.Falhas > 20 {
		return errors.New("controle de tentativas inválido; preserve o cofre e confira o arquivo .tentativas.json")
	}
	c.estado = e
	return nil
}

func (c *Tentativas) salvar() error {
	if c.caminho == "" {
		return nil
	}
	b, err := json.Marshal(c.estado)
	if err != nil {
		return err
	}
	if err := arquivo.Gravar(c.caminho, b, mycrypto.RestrictFilePermissions); err != nil {
		return fmt.Errorf("não foi possível gravar o controle de tentativas: %w", err)
	}
	return nil
}
