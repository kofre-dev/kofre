package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode"
	"unicode/utf8"

	"kofre/pkg/config"
	"kofre/pkg/conta"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/storage"
	"kofre/pkg/vault"
)

func codigoConta() ([]byte, error) {
	return segredoEmpresa("Código de oito números recebido por e-mail")
}

func cadastrarContaEmail(path string) error {
	s := contaEmUso
	i, err := abrirIdentidadeConta(path, nil)
	if errors.Is(err, os.ErrNotExist) {
		nome, e := solicitarEmpresa("Seu nome")
		if e != nil {
			return e
		}
		i, err = corporativo.PrepararCadastro(nome)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	defer i.Fechar()
	if i.ID != "" {
		informarConta("Sua conta já está cadastrada. Use Entrar por e-mail para renovar o acesso.")
		return nil
	}
	email, err := solicitarEmpresa("Seu e-mail")
	if err != nil {
		return err
	}
	i.Email, err = conta.EmailValido(email)
	if err != nil {
		return err
	}
	if err = i.ProtegerPrivada(); err != nil {
		return err
	}
	// Preserva os segredos antes da rede: resposta perdida não cria uma nova chave.
	if err = s.salvar(i); err != nil {
		return err
	}
	c, err := conta.NovoClient(config.GetCloudEndpoint())
	if err != nil {
		return err
	}
	var cifra []byte
	err = s.chave.WithBytes(func(key []byte) error {
		var e error
		cifra, e = corporativo.CifrarIdentidadeComChave(i, key, s.salt)
		return e
	})
	if err != nil {
		return err
	}
	d, err := c.Cadastrar(context.Background(), i, cifra)
	if err != nil {
		return err
	}
	informarConta("Confira sua caixa de entrada. A senha mestra permanece neste computador.")
	codigo, err := codigoConta()
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(codigo)
	dispositivo, _ := os.Hostname()
	if dispositivo == "" {
		dispositivo = "Computador"
	}
	sessao, err := c.ConfirmarCadastro(context.Background(), d, bytes.TrimSpace(codigo), i, dispositivo)
	if err != nil {
		return fmt.Errorf("confirmação não concluída neste PC; identidade local preservada. Se já confirmou, use Entrar por e-mail: %w", err)
	}
	i.ID = sessao.ID
	i.Token = sessao.Token
	if err = s.salvar(i); err != nil {
		return err
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.ContaConfigurada = true
	if err = config.SaveConfig(cfg); err != nil {
		return err
	}
	if err = publicarBackupConta(s, i); err != nil {
		return fmt.Errorf("conta criada; backup remoto pendente: %w", err)
	}
	informarConta("Conta gratuita criada e e-mail confirmado.")
	informarConta("Você continua no plano Free e usa a mesma senha mestra. Escolha a seguir se deseja sincronizar este cofre.")
	sincronizar, err := confirmarSincronizacaoConta()
	if err != nil {
		return err
	}
	if sincronizar {
		client, e := corporativo.NovoClient(config.GetCloudEndpoint(), i)
		if e != nil {
			return e
		}
		return executarAcaoEmpresa([]string{"conectar"}, path, nil, i, client)
	}
	return nil
}

func publicarBackupConta(s *sessaoConta, i *corporativo.Identidade) error {
	if i.Email == "" {
		return nil
	}
	c, err := conta.NovoClient(config.GetCloudEndpoint())
	if err != nil {
		return err
	}
	if pendente := s.cofre.BackupContaPendente(); len(pendente) > 0 {
		if err = c.EnviarBackupPendente(context.Background(), i, pendente); err != nil {
			return err
		}
		s.cofre.DefinirBackupContaPendente(nil)
		if err = s.salvar(i); err != nil {
			s.cofre.DefinirBackupContaPendente(pendente)
			return err
		}
		return nil
	}
	var cifra []byte
	err = s.chave.WithBytes(func(key []byte) error {
		var e error
		cifra, e = corporativo.CifrarIdentidadeComChave(i, key, s.salt)
		return e
	})
	if err != nil {
		return err
	}
	return c.PublicarBackup(context.Background(), i, cifra)
}

// prepararDestinoConta abre exclusivamente a cópia local, sem persistir ou
// substituir conta. O mesmo segredo informado para o backup deve abrir o cofre.
func prepararDestinoConta(raw, senha []byte, i *corporativo.Identidade) (*vault.ManagedVault, []byte, []byte, error) {
	var v *vault.ManagedVault
	var key, salt []byte
	var err error
	if len(raw) == 0 {
		v = vault.NewManaged()
		key, salt, err = mycrypto.DeriveKeyBytes(senha, nil)
	} else {
		var payload []byte
		salt, payload, err = vault.UnpackHeader(raw)
		if err == nil {
			key, _, err = mycrypto.DeriveKeyBytes(senha, salt)
		}
		if err == nil {
			v, err = vault.DecryptAndLoad(payload, key, salt)
		}
	}
	if err != nil {
		if v != nil {
			v.Close()
		}
		mycrypto.ZeroBytes(key)
		return nil, nil, nil, errors.New("a senha da conta não abriu o cofre de destino; nenhum arquivo foi alterado")
	}
	if v.TemConta() {
		if i.Pins == nil {
			i.Pins = make(map[string]string)
		}
		var anterior corporativo.Identidade
		err = v.LerConta(func(b []byte) error { return json.Unmarshal(b, &anterior) })
		if err == nil {
			a, _ := anterior.Publica()
			b, _ := i.Publica()
			if anterior.ID != "" && anterior.ID != i.ID || a != b {
				err = errors.New("cofre já vinculado a outra identidade")
			}
			if err == nil {
				for id, pin := range anterior.Pins {
					if atual := i.Pins[id]; atual != "" && atual != pin {
						err = errors.New("fingerprint do backup diverge da cópia local")
						break
					}
					i.Pins[id] = pin
				}
			}
		}
		anterior.Fechar()
	}
	if err != nil {
		v.Close()
		mycrypto.ZeroBytes(key)
		return nil, nil, nil, err
	}
	return v, key, salt, nil
}

func entrarContaEmail(recuperar bool) error {
	c, err := conta.NovoClient(config.GetCloudEndpoint())
	if err != nil {
		return err
	}
	valor, err := solicitarEmpresa("Seu e-mail")
	if err != nil {
		return err
	}
	email, err := conta.EmailValido(valor)
	if err != nil {
		return err
	}
	d, err := c.IniciarAcesso(context.Background(), email, recuperar)
	if err != nil {
		return err
	}
	codigo, err := codigoConta()
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(codigo)
	b, err := c.ConfirmarAcesso(context.Background(), d, bytes.TrimSpace(codigo))
	if err != nil {
		return err
	}
	senha, err := segredoEmpresa("Senha mestra do seu Kofre")
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(senha)
	path := resolveVaultPath("")
	raw, err := os.ReadFile(path)
	existiaLocal := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	i, err := corporativo.AbrirIdentidadeBytes(b.Cifra, senha)
	if err != nil {
		// Uma troca local cuja publicação esteja pendente continua podendo
		// provar a chave usando a identidade no próprio cofre.
		salt, payload, e := vault.UnpackHeader(raw)
		if e == nil {
			key, _, e := mycrypto.DeriveKeyBytes(senha, salt)
			if e == nil {
				local, e := vault.DecryptAndLoad(payload, key, salt)
				if e == nil {
					var identidade corporativo.Identidade
					e = local.LerConta(func(data []byte) error { return json.Unmarshal(data, &identidade) })
					local.Close()
					if e == nil && identidade.ID == b.ID {
						i = &identidade
						err = nil
					} else {
						identidade.Fechar()
					}
				}
				mycrypto.ZeroBytes(key)
			}
		}
		if err != nil {
			return errors.New("senha mestra não abriu sua identidade; o e-mail não redefine essa senha")
		}
	}
	defer i.Fechar()
	if i.ID != "" && i.ID != b.ID {
		return errors.New("backup de outra conta recusado")
	}
	i.ID = b.ID
	i.Email = email
	if err = i.ProtegerPrivada(); err != nil {
		return err
	}
	v, key, salt, err := prepararDestinoConta(raw, senha, i)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(key)
	defer v.Close()
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.CloudEnabled && (cfg.Mode == "custom_s3" || cfg.ContaID != i.ID) {
		return errors.New("outra nuvem está configurada; desvincule-a antes de entrar nesta conta")
	}
	nome, _ := os.Hostname()
	if nome == "" {
		nome = "Computador"
	}
	sessao, err := c.Concluir(context.Background(), d, b, i, nome)
	if err != nil {
		return err
	}
	i.Token = sessao.Token
	confirmada := false
	defer func() {
		if !confirmada {
			_ = c.Logout(context.Background(), sessao.Token)
		}
	}()
	local, err := storage.NewLocalStorage(path)
	if err != nil {
		return err
	}
	cloud, err := nuvemContaComRevisao(path, i)
	if err != nil {
		return err
	}
	var baseRemotaValidada []byte
	// Destino vazio pode recuperar o cofre remoto. Destino com entradas permanece
	// intacto e exige resolução explícita de conflitos pelo mecanismo de sync.
	if v.Count() == 0 {
		remoto, e := cloud.Load(context.Background())
		if e == nil {
			av, k, sa, e := prepararDestinoConta(remoto, senha, i)
			if e != nil {
				return e
			}
			defer av.Close()
			defer mycrypto.ZeroBytes(k)
			baseRemotaValidada = remoto
			v = av
			key = k
			salt = sa
		} else if !errors.Is(e, storage.ErrNotFound) {
			return e
		}
	}
	data, err := i.Serializar()
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(data)
	if err = v.DefinirConta(data); err != nil {
		return err
	}
	packed, err := v.Pack(key, salt)
	if err != nil {
		return err
	}
	// Confere a base e preserva backup sob o mesmo lock que protege a
	// substituição; outro cliente não pode gravar entre a conferência e o save.
	if _, err = local.SubstituirSeIgual(context.Background(), raw, existiaLocal, packed); err != nil {
		return err
	}
	confirmada = true
	if baseRemotaValidada != nil {
		// O conteúdo baixado foi autenticado antes de gerar e instalar packed.
		// Reconhece essa base somente agora; o próximo envio continua com CAS.
		if err = cloud.ConfirmarLeitura(baseRemotaValidada); err != nil {
			return fmt.Errorf("conta salva localmente; revisão da nuvem pendente: %w", err)
		}
	}
	// A sessão foi salva no cofre. Uma falha abaixo não é um rollback do login.
	if err = config.AtivarLicencaComprada(config.GetCloudEndpoint(), i.Token); err != nil {
		return fmt.Errorf("acesso salvo no cofre; conexão da nuvem pendente: %w", err)
	}
	informarConta("Conta conectada. A sessão deste computador tem validade de 30 dias.")
	if err = cloud.Save(context.Background(), packed); err != nil {
		informarConta("Cofre local preservado. Sincronização pendente; confira os conflitos antes de enviar.")
	}
	return nil
}

// A primeira sincronização da conta precisa persistir a mesma base que será
// usada ao reabrir a TUI; confirmar apenas em RAM cria conflitos no reinício.
func nuvemContaComRevisao(path string, i *corporativo.Identidade) (*storage.KofreCloudStorage, error) {
	cloud := storage.NewKofreCloudStorage(config.GetCloudEndpoint(), i.Token, i.ID)
	if err := cloud.DefinirArquivoRevisao(path + ".cloud-revision.json"); err != nil {
		return nil, err
	}
	return cloud, nil
}

func administrarSessoesConta(i *corporativo.Identidade) error {
	c, err := conta.NovoClient(config.GetCloudEndpoint())
	if err != nil {
		return err
	}
	sessoes, err := c.Sessoes(context.Background(), i.Token)
	if err != nil {
		return err
	}
	opcoes := make([]string, len(sessoes))
	for n, s := range sessoes {
		opcoes[n] = textoEmpresa(s.Nome) + " · até " + s.ExpiraEm.Local().Format("02/01/2006")
		if s.Atual {
			opcoes[n] += " · este computador"
		}
	}
	if len(opcoes) == 0 {
		informarConta("Nenhuma sessão por dispositivo. Entre por e-mail neste computador.")
		return nil
	}
	n, err := escolherOpcaoConta("Sessões · escolha para encerrar", opcoes)
	if err != nil || n < 0 {
		return err
	}
	if !confirmarEmpresa("Digite ENCERRAR para revogar o acesso desse dispositivo", "ENCERRAR") {
		return nil
	}
	if err = c.Revogar(context.Background(), i.Token, sessoes[n].ID); err != nil {
		return err
	}
	if sessoes[n].Atual {
		return encerrarContaLocal(i)
	}
	informarConta("Sessão encerrada. O cofre local desse dispositivo continua cifrado.")
	return nil
}

func encerrarContaLocal(i *corporativo.Identidade) error {
	i.Token = ""
	if err := salvarIdentidadeConta(contaEmUso.identidade, i, nil); err != nil {
		return err
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.ContaID == i.ID {
		cfg.CloudEnabled = false
		cfg.KofreToken = ""
		cfg.PlanoCloud = "free"
		return config.SaveConfig(cfg)
	}
	return nil
}

// Revelação explícita sem materializar uma string imutável nem aceitar escapes
// do servidor. O terminal pode guardar sua própria cópia; apague o buffer usado.
func escreverSegredoTerminal(data []byte) error {
	if interfaceConta != nil {
		protegido, err := mycrypto.SealMemory(data)
		mycrypto.ZeroBytes(data)
		if err != nil {
			return err
		}
		defer protegido.Close()
		return interfaceConta.Revelar("Credencial compartilhada", protegido.WithBytes)
	}
	return escreverSegredoPara(os.Stdout, data)
}
func escreverSegredoPara(dest io.Writer, data []byte) error {
	out := make([]byte, 0, 3*len(data)+1)
	defer mycrypto.ZeroBytes(out[:cap(out)])
	for len(data) > 0 {
		r, n := utf8.DecodeRune(data)
		data = data[n:]
		if !unicode.IsControl(r) {
			out = utf8.AppendRune(out, r)
		}
	}
	out = append(out, '\n')
	_, err := dest.Write(out)
	return err
}
