package runner

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/term"

	"kofre/pkg/acesso"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

// UnlockVault carrega o arquivo do cofre e solicita a senha/PIN de forma segura sem eco
func UnlockVault(vaultPath string) (*vault.ManagedVault, error) {
	v, key, _, err := UnlockVaultWithKey(vaultPath)
	if key != nil {
		mycrypto.ZeroBytes(key)
	}
	return v, err
}

// UnlockVaultWithKey abre o cofre e retorna tambem a chave de sessao e o salt para regravacao
func UnlockVaultWithKey(vaultPath string) (*vault.ManagedVault, []byte, []byte, error) {
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		return nil, nil, nil, fmt.Errorf("arquivo do cofre nao encontrado em %s (inicie o kofre para criar o primeiro)", vaultPath)
	}

	rawData, err := os.ReadFile(vaultPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("falha ao ler cofre: %w", err)
	}

	return UnlockVaultData(rawData, vaultPath)
}

// UnlockVaultData autentica uma cópia candidata em memória, sem instalá-la no
// disco. Usado também em downloads para validar antes de substituir o local.
func UnlockVaultData(rawData []byte, vaultPath string) (*vault.ManagedVault, []byte, []byte, error) {
	salt, encryptedPayload, err := vault.UnpackHeader(rawData)
	if err != nil {
		return nil, nil, nil, err
	}
	tentativa, err := acesso.Novas(vaultPath).Iniciar(time.Now())
	if err != nil {
		return nil, nil, nil, err
	}
	defer tentativa.Fechar()

	secret := []byte(os.Getenv("KOFRE_PIN"))
	if len(secret) == 0 {
		secret = []byte(os.Getenv("MYCOFRE_PIN"))
	}
	if len(secret) == 0 {
		fmt.Print("🔐 Kofre — Digite seu PIN ou chave mestra: ")
		bytePass, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			mycrypto.ZeroBytes(bytePass)
			return nil, nil, nil, fmt.Errorf("falha ao ler entrada segura: %w", err)
		}
		secret = bytePass
	}
	defer mycrypto.ZeroBytes(secret)
	secretNormalizado := bytes.TrimSpace(secret)
	if len(secretNormalizado) == 0 {
		return nil, nil, nil, errors.New("chave ou PIN nao informado")
	}

	key, _, err := mycrypto.DeriveKeyBytes(secretNormalizado, salt)
	if err != nil {
		return nil, nil, nil, err
	}

	v, err := vault.DecryptAndLoad(encryptedPayload, key, salt)
	if err != nil {
		mycrypto.ZeroBytes(key)
		return nil, nil, nil, fmt.Errorf("PIN ou chave mestra incorreta")
	}
	if err := tentativa.Sucesso(); err != nil {
		v.Close()
		mycrypto.ZeroBytes(key)
		return nil, nil, nil, err
	}

	return v, key, salt, nil
}

// RunExec executa um comando especifico injetando as credenciais no processo filho
func RunExec(cmdArgs []string, filterEntry, vaultPath string, fields ...string) int {
	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "Erro: nenhum comando especificado para execucao.")
		return 1
	}

	childEnv, _, err := prepararAmbiente(vaultPath, filterEntry, fields)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao exportar credenciais: %v\n", err)
		return 1
	}

	// Prepara comando filho
	cmdName := cmdArgs[0]
	var args []string
	if len(cmdArgs) > 1 {
		args = cmdArgs[1:]
	}

	cmd := exec.Command(cmdName, args...)
	cmd.Env = childEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Encaminha sinais de interrupcao (Ctrl+C) para o processo filho
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	if err := cmd.Start(); err != nil {
		cmd.Env = nil
		limparAmbiente(childEnv)
		fmt.Fprintf(os.Stderr, "Falha ao executar comando: %v\n", err)
		return 1
	}
	cmd.Env = nil
	limparAmbiente(childEnv)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-sigChan:
				_ = cmd.Process.Signal(sig)
			case <-done:
				return
			}
		}
	}()

	err = cmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "Falha ao executar comando: %v\n", err)
		return 1
	}

	return 0
}

// RunShell abre uma nova sessao de shell interativa com as credenciais injetadas
func RunShell(ttl time.Duration, filterEntry, vaultPath string, fields ...string) int {
	childEnv, quantidade, err := prepararAmbiente(vaultPath, filterEntry, fields)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao exportar credenciais: %v\n", err)
		return 1
	}

	var shellName string
	var shellArgs []string

	if runtime.GOOS == "windows" {
		shellName = "powershell.exe"
		shellArgs = []string{"-NoLogo"}
	} else {
		shellName = os.Getenv("SHELL")
		if shellName == "" {
			shellName = "/bin/bash"
		}
	}

	ttlInfo := "sem limite (encerra ao fechar)"
	if ttl > 0 {
		ttlInfo = fmt.Sprintf("%s (encerra este shell)", ttl.String())
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ 🔐 Kofre Shell Ativo — Variáveis de Ambiente Injetadas    │")
	fmt.Printf("│ • Injetadas:  %d variáveis selecionadas                     │\n", quantidade)
	fmt.Printf("│ • TTL Sessão: %-46s│\n", ttlInfo)
	fmt.Println("│ • O shell e seus filhos podem ler as variáveis selecionadas │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()

	cmd := exec.Command(shellName, shellArgs...)
	cmd.Env = childEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cmd.Env = nil
		limparAmbiente(childEnv)
		fmt.Fprintf(os.Stderr, "Falha ao iniciar shell: %v\n", err)
		return 1
	}
	cmd.Env = nil
	limparAmbiente(childEnv)
	done := make(chan struct{})
	defer close(done)
	// O TTL encerra este shell; não revoga dados já lidos por seus descendentes.
	if ttl > 0 {
		go func() {
			timer := time.NewTimer(ttl)
			defer timer.Stop()
			select {
			case <-timer.C:
				fmt.Println("\n⚠️  [Kofre] Tempo de vida (TTL) da sessão expirou! Encerrando...")
				_ = cmd.Process.Kill()
			case <-done:
			}
		}()
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	fmt.Println("\n🔒 [Kofre] Sessão encerrada.")
	return 0
}
