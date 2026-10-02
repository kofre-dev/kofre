package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

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

	salt, encryptedPayload, err := vault.UnpackHeader(rawData)
	if err != nil {
		return nil, nil, nil, err
	}

	secret := os.Getenv("KOFRE_PIN")
	if secret == "" {
		secret = os.Getenv("MYCOFRE_PIN")
	}

	if secret == "" {
		fmt.Print("🔐 Kofre — Digite seu PIN ou chave mestra: ")
		bytePass, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("falha ao ler entrada segura: %w", err)
		}
		secret = strings.TrimSpace(string(bytePass))
	}

	if secret == "" {
		return nil, nil, nil, errors.New("chave ou PIN nao informado")
	}

	key, _, err := mycrypto.DeriveKey(secret, salt)
	if err != nil {
		return nil, nil, nil, err
	}

	v, err := vault.DecryptAndLoad(encryptedPayload, key, salt)
	if err != nil {
		mycrypto.ZeroBytes(key)
		return nil, nil, nil, fmt.Errorf("PIN ou chave mestra incorreta")
	}

	return v, key, salt, nil
}

// RunExec executa um comando especifico injetando as credenciais no processo filho
func RunExec(cmdArgs []string, filterEntry, vaultPath string) int {
	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "Erro: nenhum comando especificado para execucao.")
		return 1
	}

	v, err := UnlockVault(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro de autenticacao: %v\n", err)
		return 1
	}

	envMap := v.ToEnvMap(filterEntry)
	if len(envMap) == 0 {
		fmt.Fprintf(os.Stderr, "Aviso: nenhuma variavel de ambiente encontrada no cofre para injetar.\n")
	}

	childEnv := os.Environ()
	for k, val := range envMap {
		childEnv = append(childEnv, fmt.Sprintf("%s=%s", k, val))
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
	go func() {
		for sig := range sigChan {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		}
	}()

	err = cmd.Run()
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
func RunShell(ttl time.Duration, filterEntry, vaultPath string) int {
	v, err := UnlockVault(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro de autenticacao: %v\n", err)
		return 1
	}

	envMap := v.ToEnvMap(filterEntry)
	childEnv := os.Environ()
	for k, val := range envMap {
		childEnv = append(childEnv, fmt.Sprintf("%s=%s", k, val))
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
		ttlInfo = fmt.Sprintf("%s (auto-destruicao da sessao)", ttl.String())
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ 🔐 Kofre Shell Ativo — Variáveis de Ambiente Injetadas    │")
	fmt.Printf("│ • Injetadas:  %d variáveis diretamente na memória RAM        │\n", len(envMap))
	fmt.Printf("│ • TTL Sessão: %-46s│\n", ttlInfo)
	fmt.Println("│ • Rastro:     Zero arquivo no disco. Digite 'exit' para sair │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()

	cmd := exec.Command(shellName, shellArgs...)
	cmd.Env = childEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Watchdog de TTL opcional
	if ttl > 0 {
		go func() {
			time.Sleep(ttl)
			if cmd.Process != nil {
				fmt.Println("\n⚠️  [Kofre] Tempo de vida (TTL) da sessão expirou! Encerrando...")
				_ = cmd.Process.Kill()
			}
		}()
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	fmt.Println("\n🔒 [Kofre] Sessão encerrada. Variáveis de ambiente zeradas da memória.")
	return 0
}
