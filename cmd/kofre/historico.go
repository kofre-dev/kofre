package main

import (
	"bytes"
	"context"
	"fmt"
	"kofre/pkg/config"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/runner"
	"kofre/pkg/storage"
	"kofre/pkg/tui"
	"kofre/pkg/vault"
	"time"
)

func executarHistorico(args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.CloudEnabled || cfg.KofreToken == "" {
		return fmt.Errorf("conecte sua conta antes de consultar o histórico")
	}
	cloud := storage.NewKofreCloudStorage(config.GetCloudEndpoint(), cfg.KofreToken, cfg.ContaID)
	ctx := context.Background()
	if len(args) == 1 && args[0] == "painel" {
		versoes, err := cloud.Historico(ctx)
		if err != nil {
			return err
		}
		if len(versoes) == 0 {
			fmt.Println("Ainda não há versões anteriores guardadas.")
			return nil
		}
		nomes := make([]string, len(versoes))
		for i, v := range versoes {
			nomes[i] = v.CriadaEm.Local().Format("02/01/2006 15:04:05")
		}
		i, err := tui.EscolherOpcao("Escolha uma versão para restaurar", nomes)
		if err != nil || i < 0 {
			return err
		}
		args = []string{"restaurar", versoes[i].ID}
	}
	if len(args) == 0 {
		versoes, err := cloud.Historico(ctx)
		if err != nil {
			return err
		}
		fmt.Println("Histórico cifrado do seu cofre pessoal")
		for _, v := range versoes {
			fmt.Printf("%s  %s\n", v.CriadaEm.Local().Format("02/01/2006 15:04:05"), v.ID)
		}
		if len(versoes) == 0 {
			fmt.Println("Ainda não há versões anteriores guardadas.")
		} else {
			fmt.Println("Para restaurar: kofre historico restaurar <versão>. A senha usada nessa versão será necessária.")
		}
		return nil
	}
	if len(args) != 2 || args[0] != "restaurar" {
		return fmt.Errorf("use kofre historico ou kofre historico restaurar <versão>")
	}
	data, err := cloud.BaixarVersao(ctx, args[1])
	if err != nil {
		return err
	}
	salt, payload, err := vault.UnpackHeader(data)
	if err != nil {
		return err
	}
	senha, err := segredoEmpresa("Senha mestra desta versão do cofre")
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(senha)
	key, _, err := mycrypto.DeriveKeyBytes(senha, salt)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(key)
	aberto, err := vault.DecryptAndLoad(payload, key, salt)
	if err != nil {
		return fmt.Errorf("versão não validada; nenhum arquivo foi alterado: %w", err)
	}
	defer aberto.Close()
	if perguntarEmpresa("Restaurar esta versão e preservar backup do cofre atual? Digite RESTAURAR") != "RESTAURAR" {
		return nil
	}
	local, err := storage.NewLocalStorage(resolveVaultPath(""))
	if err != nil {
		return err
	}
	// Exige que a cópia local seja a atual. Não descarta edições pendentes.
	remoto, err := cloud.Load(ctx)
	if err != nil {
		return err
	}
	atual, err := local.Load(ctx)
	if err != nil {
		return err
	}
	if !bytes.Equal(remoto, atual) {
		return fmt.Errorf("há diferenças entre este PC e a nuvem; preserve as duas cópias e use kofre pull antes de restaurar")
	}
	// Restaurar credenciais não pode reverter tokens, convites ou chaves da conta.
	fmt.Println("Confirme a senha mestra atual para preservar a conta nesta restauração.")
	cofreAtual, chaveAtual, _, err := runner.UnlockVaultWithKey(resolveVaultPath(""))
	mycrypto.ZeroBytes(chaveAtual)
	if err != nil {
		return err
	}
	defer cofreAtual.Close()
	if cofreAtual.TemConta() {
		if err = cofreAtual.LerConta(aberto.DefinirConta); err != nil {
			return err
		}
		data, err = aberto.Pack(key, salt)
		if err != nil {
			return err
		}
	}
	conferido, err := local.Load(ctx)
	if err != nil || !bytes.Equal(conferido, atual) {
		return fmt.Errorf("cofre local mudou; repita a restauração")
	}
	sync := storage.NewSyncStorage(local, cloud)
	defer sync.Close()
	// NewSyncStorage vincula a revisão persistente; confirmar somente a cópia
	// que já está instalada. O envio continua sujeito a conflito entre PCs.
	remoto, err = cloud.Load(ctx)
	if err != nil || !bytes.Equal(remoto, atual) {
		return fmt.Errorf("cofre remoto mudou; repita a restauração")
	}
	if err := cloud.ConfirmarLeitura(atual); err != nil {
		return err
	}
	backup, err := local.PreservarBackup(ctx, data)
	if err != nil {
		return err
	}
	if backup != "" {
		fmt.Println("Backup cifrado preservado em:", backup)
	}
	if err := sync.Save(ctx, data); err != nil {
		return err
	}
	if err := sync.Flush(30 * time.Second); err != nil {
		return fmt.Errorf("versão restaurada localmente; envio pendente e backup preservado: %w", err)
	}
	fmt.Println("Versão restaurada no computador e na nuvem. Use a senha mestra dessa versão ao abrir.")
	return nil
}
