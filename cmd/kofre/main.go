package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"kofre/pkg/config"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/importer"
	"kofre/pkg/installer"
	"kofre/pkg/runner"
	"kofre/pkg/storage"
	"kofre/pkg/tui"
	"kofre/pkg/updater"
	"kofre/pkg/vault"
)

func printHelp() {
	fmt.Printf("Kofre 🔐 — Cofre Criptografado Portátil e Zero-Knowledge v%s (https://kofre.dev)\n\n", updater.CurrentVersion)
	fmt.Println(`Comandos Principais:
  kofre                   Abre o cofre na interface visual interativa (TUI)
  kofre exec <cmd...>     Injeta segredos na memória e executa o comando filho
  kofre shell             Abre um subshell interativo temporário com segredos
  kofre install           Instala no seu sistema e adiciona ao PATH do terminal
  kofre config            Assistente interativo de configuração (S3, Cloud, Telegram)
  kofre login <token>     Conecta sua licença Kofre Cloud Pro para sync automático
  kofre telegram          Vincula seu cofre ao Bot do Telegram (Alertas e Pânico)
  kofre push              Envia o cofre local criptografado para o S3 / Cloud
  kofre pull              Baixa o cofre mais recente do S3 / Cloud para o PC
  kofre import <txt>      Importador inteligente de arquivos .txt desformatados
  kofre update            Verifica e aplica atualizações mais recentes do binário
  kofre pro               Ativa a assinatura Kofre Cloud Pro (Modo Piloto)
  kofre version           Exibe a versão do executável Kofre
  kofre backup [pasta]    Gera uma cópia de backup criptografada com timestamp
  kofre restore <arquivo> Restaura o cofre a partir de um backup validado
  kofre info              Exibe os caminhos e status do cofre atual

Injeção Segura de Ambiente (Sem arquivos .env no disco):
  kofre exec python main.py
  kofre exec npm run dev
  kofre exec --only "AWS" -- aws s3 ls
  kofre shell --ttl 30m

Opções Globais (Flags):
  --vault <caminho>         Especifica um arquivo de cofre personalizado
  --s3-bucket <bucket>      Conecta diretamente ao bucket AWS S3 / Cloudflare R2
  --s3-key <chave>          Nome do arquivo no S3 (padrão: vault.enc)
  --s3-region <regiao>      Região do bucket (padrão: us-east-1)
  --s3-endpoint <url>       Endpoint S3 personalizado (para Cloudflare R2 ou MinIO)
  -h, --help                Exibe esta mensagem de ajuda`)
}

func main() {
	updater.CleanupOldBinaries()
	mycrypto.PurgeLegacyDeviceKey()

	if len(os.Args) > 1 {
		cmd := os.Args[1]
		if cmd != "update" && cmd != "version" && cmd != "-v" && cmd != "--version" && cmd != "-h" && cmd != "--help" && cmd != "help" {
			tryAutoUpdateOnBoot()
		}

		switch cmd {
		case "-h", "--help", "help":
			printHelp()
			return

		case "version", "-v", "--version":
			fmt.Printf("Kofre v%s (%s)\n", updater.CurrentVersion, updater.CurrentPlatform())
			return

		case "update":
			handleUpdate()
			return

		case "pro":
			handlePro()
			return

		case "exec":
			handleExec(os.Args[2:])
			return

		case "shell":
			handleShell(os.Args[2:])
			return

		case "install":
			dest, err := installer.InstallBinary()
			if err != nil {
				fmt.Printf("⚠️  Aviso: %v\n", err)
			} else {
				fmt.Printf("✓ Kofre instalado com sucesso em:\n  %s\n\n", dest)
				fmt.Println("O diretório foi adicionado ao seu PATH.")
				fmt.Println("Abra um novo terminal e digite 'kofre' de qualquer lugar!")
			}
			return

		case "config":
			if err := config.RunInteractiveConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
				os.Exit(1)
			}
			return

		case "login":
			handleLogin(os.Args[2:])
			return

		case "telegram", "tg":
			handleTelegramLink()
			return

		case "import":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Uso: kofre import <caminho-do-arquivo-ou-pasta> [--yes]")
				os.Exit(1)
			}
			srcFile := os.Args[2]
			autoConfirm := false
			for _, a := range os.Args[3:] {
				if a == "--yes" || a == "-y" {
					autoConfirm = true
				}
			}
			vaultPath := resolveVaultPath("")
			activeStore, err := resolveActiveStorageProvider("", "", "", "", "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erro no storage: %v\n", err)
				os.Exit(1)
			}
			if err := importer.RunInteractiveImport(srcFile, vaultPath, activeStore, autoConfirm); err != nil {
				fmt.Fprintf(os.Stderr, "Erro na importação: %v\n", err)
				os.Exit(1)
			}
			return

		case "backup":
			destDir := ""
			if len(os.Args) > 2 {
				destDir = os.Args[2]
			}
			vaultPath := resolveVaultPath("")
			backupFile, err := installer.CreateBackup(vaultPath, destDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erro ao gerar backup: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Backup seguro gerado com sucesso:\n  %s\n", backupFile)
			return

		case "restore":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Uso: Kofre restore <caminho-do-backup.enc>")
				os.Exit(1)
			}
			backupFile := os.Args[2]
			targetVault := resolveVaultPath("")
			if err := installer.RestoreBackup(backupFile, targetVault); err != nil {
				fmt.Fprintf(os.Stderr, "Erro ao restaurar backup: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Cofre restaurado com sucesso em:\n  %s\n", targetVault)
			return

		case "info":
			vaultPath := resolveVaultPath("")
			appDir, _ := config.GetDefaultDir()
			fmt.Printf("Kofre 🔐 Informações do Sistema:\n")
			fmt.Printf("  • Versão:         v%s (%s)\n", updater.CurrentVersion, updater.CurrentPlatform())
			fmt.Printf("  • Pasta de Dados: %s\n", appDir)
			fmt.Printf("  • Arquivo Ativo:  %s\n", vaultPath)
			if info, err := os.Stat(vaultPath); err == nil {
				fmt.Printf("  • Tamanho:        %d bytes\n", info.Size())
				fmt.Printf("  • Modificado em:  %s\n", info.ModTime().Format("02/01/2006 15:04:05"))
			} else {
				fmt.Printf("  • Status:         Não inicializado ainda\n")
			}
			return
		case "push":
			handlePush()
			return

		case "pull":
			handlePull()
			return
		}
	}

	tryAutoUpdateOnBoot()

	// Flags normais para abertura da TUI
	vaultFlag := flag.String("vault", "", "Caminho do arquivo local do cofre")
	s3Bucket := flag.String("s3-bucket", "", "Nome do bucket S3")
	s3Key := flag.String("s3-key", "vault.enc", "Chave do arquivo no S3")
	s3Region := flag.String("s3-region", "us-east-1", "Região do S3")
	s3Endpoint := flag.String("s3-endpoint", "", "Endpoint S3 personalizado (R2/MinIO)")
	flag.Parse()

	store, err := resolveActiveStorageProvider(*vaultFlag, *s3Bucket, *s3Key, *s3Region, *s3Endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao inicializar armazenamento: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(tui.NewModel(store), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro na execução do Kofre: %v\n", err)
		os.Exit(1)
	}
	if syncer, ok := store.(interface{ Flush(time.Duration) }); ok {
		syncer.Flush(3 * time.Second)
	}
}

func handlePush() {
	cfg, _ := config.LoadConfig()
	s3Store, err := resolveCloudOrS3Storage(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro: Armazenamento em nuvem não configurado. Execute 'kofre config' ou 'kofre login' primeiro.\n")
		os.Exit(1)
	}

	vaultPath := resolveVaultPath("")
	data, err := os.ReadFile(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao ler cofre local: %v\n", err)
		os.Exit(1)
	}

	// Valida integridade do cabeçalho
	if _, _, err := vault.UnpackHeader(data); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: arquivo local corrompido ou inválido: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	fmt.Printf("Sincronizando com a nuvem (%s)...\n", s3Store.Location())
	if err := s3Store.Save(ctx, data); err != nil {
		fmt.Fprintf(os.Stderr, "Falha no envio para o storage: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Cofre criptografado enviado com sucesso para: %s\n", s3Store.Location())
}

func handlePull() {
	cfg, _ := config.LoadConfig()
	s3Store, err := resolveCloudOrS3Storage(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro: Armazenamento em nuvem não configurado. Execute 'kofre config' ou 'kofre login' primeiro.\n")
		os.Exit(1)
	}

	ctx := context.Background()
	fmt.Printf("Baixando cofre da nuvem (%s)...\n", s3Store.Location())
	data, err := s3Store.Load(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Falha ao baixar da nuvem: %v\n", err)
		os.Exit(1)
	}

	if _, _, err := vault.UnpackHeader(data); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: arquivo na nuvem não é um cofre Kofre válido: %v\n", err)
		os.Exit(1)
	}

	vaultPath := resolveVaultPath("")
	localStore, err := storage.NewLocalStorage(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro local: %v\n", err)
		os.Exit(1)
	}

	if err := localStore.Save(ctx, data); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao gravar arquivo baixado: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Cofre baixado e sincronizado com sucesso em: %s\n", vaultPath)
}

func handleLogin(args []string) {
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	endpoint := config.GetCloudEndpoint()

	if len(args) == 0 {
		handleTelegramReverseLogin(cfg, endpoint)
		return
	}

	token := strings.TrimSpace(args[0])
	if len(args) > 1 {
		endpoint = strings.TrimSpace(args[1])
	}

	cfg.Mode = "kofre_cloud"
	cfg.CloudEnabled = true
	cfg.KofreToken = token
	cfg.CloudEndpoint = endpoint

	if err := config.SaveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao salvar configuração: %v\n", err)
		os.Exit(1)
	}

	cloudStore := storage.NewKofreCloudStorage(cfg.CloudEndpoint, cfg.KofreToken)
	fmt.Println("✓ Conectado ao Kofre Cloud!")
	fmt.Printf("  • Endpoint: %s\n", cloudStore.Location())
	fmt.Println("Execute 'kofre pull' para baixar seu cofre ou 'kofre' para abrir na interface.")
}

func handleTelegramReverseLogin(cfg *config.AppConfig, endpoint string) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  Kofre 🔐 — Conexão Rápida em Novo Computador               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Computador"
	}
	deviceName := fmt.Sprintf("%s (%s)", hostname, updater.CurrentPlatform())

	payload, _ := json.Marshal(map[string]string{
		"device_name": deviceName,
	})
	initURL := fmt.Sprintf("%s/v1/auth/telegram-login/init", strings.TrimRight(endpoint, "/"))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(initURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Falha ao conectar com Kofre Cloud: %v\n", err)
		fmt.Println("Alternativa: Você pode usar 'kofre login <token>' com seu token de licença.")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Erro da API (%d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var initRes struct {
		SessionID        string `json:"session_id"`
		Code             string `json:"code"`
		FormattedCode    string `json:"formatted_code"`
		Bot              string `json:"bot"`
		LoginURL         string `json:"login_url"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&initRes); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao decodificar resposta: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Para conectar este computador instantaneamente sem copiar nenhum token:")
	fmt.Println()
	fmt.Println("1. Abra o link de autorização no Telegram do seu celular:")
	fmt.Printf("   👉 %s\n\n", initRes.LoginURL)
	fmt.Printf("   Ou envie o código direto para @%s:\n", initRes.Bot)
	fmt.Printf("   🔑 CÓDIGO: %s\n\n", initRes.FormattedCode)
	fmt.Println("2. Toque em [ ✅ Autorizar Novo PC ] na mensagem que você receber.")
	fmt.Println()
	fmt.Print("⏳ Aguardando aprovação no seu celular")

	statusURL := fmt.Sprintf("%s/v1/auth/telegram-login/status?session_id=%s", strings.TrimRight(endpoint, "/"), initRes.SessionID)
	deadline := time.Now().Add(5 * time.Minute)
	pollClient := &http.Client{Timeout: 5 * time.Second}

	approvedToken := ""
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		fmt.Print(".")

		req, err := http.NewRequest(http.MethodGet, statusURL, nil)
		if err != nil {
			continue
		}
		sResp, err := pollClient.Do(req)
		if err != nil {
			continue
		}

		var sData struct {
			Status string `json:"status"`
			Token  string `json:"token"`
		}
		_ = json.NewDecoder(sResp.Body).Decode(&sData)
		sResp.Body.Close()

		if sData.Status == "approved" && sData.Token != "" {
			approvedToken = sData.Token
			break
		}
		if sData.Status == "rejected" {
			fmt.Println("\n\n🚨 Acesso recusado pelo usuário no Telegram.")
			os.Exit(1)
		}
		if sData.Status == "expired" {
			fmt.Println("\n\n⏱️ Sessão expirada. Tente novamente executando 'kofre login'.")
			os.Exit(1)
		}
	}

	if approvedToken == "" {
		fmt.Println("\n\n⏱️ Tempo limite excedido (5 minutos). Execute 'kofre login' para tentar novamente.")
		os.Exit(1)
	}

	fmt.Println("\n\n✓ Autorização concedida com sucesso via Telegram!")

	// Salva a configuração localmente com proteção DPAPI automática
	cfg.Mode = "kofre_cloud"
	cfg.CloudEnabled = true
	cfg.KofreToken = approvedToken
	cfg.CloudEndpoint = endpoint

	if err := config.SaveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao salvar configuração: %v\n", err)
		os.Exit(1)
	}

	// Baixa o cofre criptografado imediatamente
	fmt.Println("Sincronizando seu cofre com a nuvem...")
	handlePull()

	fmt.Println()
	fmt.Println("🎉 Tudo pronto! Este computador está pareado e protegido.")
	fmt.Println("Digite 'kofre' para abrir seu cofre.")
}

func resolveActiveStorageProvider(vaultFlag, flagBucket, flagKey, flagRegion, flagEndpoint string) (storage.StorageProvider, error) {
	finalPath := resolveVaultPath(vaultFlag)
	localStore, err := storage.NewLocalStorage(finalPath)
	if err != nil {
		return nil, err
	}

	// 1. Se informou bucket via flag
	if flagBucket != "" {
		s3Store := storage.NewS3Storage(storage.S3Config{
			Bucket:          flagBucket,
			Key:             flagKey,
			Region:          flagRegion,
			Endpoint:        flagEndpoint,
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		})
		return storage.NewSyncStorage(localStore, s3Store), nil
	}

	// 2. Se informou bucket via variavel de ambiente
	envBucket := os.Getenv("AWS_S3_BUCKET")
	if envBucket != "" {
		ep := flagEndpoint
		if ep == "" {
			ep = os.Getenv("AWS_ENDPOINT_URL")
		}
		reg := flagRegion
		if os.Getenv("AWS_REGION") != "" {
			reg = os.Getenv("AWS_REGION")
		}
		s3Store := storage.NewS3Storage(storage.S3Config{
			Bucket:          envBucket,
			Key:             flagKey,
			Region:          reg,
			Endpoint:        ep,
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		})
		return storage.NewSyncStorage(localStore, s3Store), nil
	}

	// 3. Se configurado via 'kofre config' ou 'kofre login'
	cfg, err := config.LoadConfig()
	if err == nil && cfg.CloudEnabled {
		if remoteStore, err := resolveCloudOrS3Storage(cfg); err == nil {
			return storage.NewSyncStorage(localStore, remoteStore), nil
		}
	}

	// 4. Modo padrao: Apenas Local
	return localStore, nil
}

func resolveCloudOrS3Storage(cfg *config.AppConfig) (storage.StorageProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuração não encontrada")
	}

	// 1. Modo Kofre Cloud
	if cfg.Mode == "kofre_cloud" || (cfg.CloudEnabled && cfg.KofreToken != "") {
		endpoint := config.GetCloudEndpoint()
		return storage.NewKofreCloudStorage(endpoint, cfg.KofreToken), nil
	}

	// 2. Modo S3 / R2 Próprio
	if cfg.S3Bucket != "" {
		ak := cfg.S3AccessKey
		if ak == "" {
			ak = os.Getenv("AWS_ACCESS_KEY_ID")
		}
		sk := cfg.S3SecretKey
		if sk == "" {
			sk = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}
		s3Key := cfg.S3Key
		if s3Key == "" {
			s3Key = "vault.enc"
		}
		region := cfg.S3Region
		if region == "" {
			region = "sa-east-1"
		}
		return storage.NewS3Storage(storage.S3Config{
			Bucket:          cfg.S3Bucket,
			Key:             s3Key,
			Region:          region,
			Endpoint:        cfg.S3Endpoint,
			AccessKeyID:     ak,
			SecretAccessKey: sk,
		}), nil
	}

	return nil, fmt.Errorf("nenhum armazenamento em nuvem configurado")
}

func handleExec(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`Uso: Kofre exec [--only <filtro>] [--vault <caminho>] -- <comando> [argumentos...]

Exemplos:
  Kofre exec python main.py
  Kofre exec npm run dev
  Kofre exec --only "AWS" -- aws s3 ls`)
		return
	}

	filterEntry := ""
	customVault := ""
	var cmdArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--only" && i+1 < len(args) {
			filterEntry = args[i+1]
			i++
		} else if arg == "--vault" && i+1 < len(args) {
			customVault = args[i+1]
			i++
		} else if arg == "--" {
			cmdArgs = args[i+1:]
			break
		} else if !stringsHasPrefix(arg, "-") {
			cmdArgs = args[i:]
			break
		}
	}

	vaultPath := resolveVaultPath(customVault)
	code := runner.RunExec(cmdArgs, filterEntry, vaultPath)
	os.Exit(code)
}

func handleShell(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Println(`Uso: Kofre shell [--ttl <tempo>] [--only <filtro>] [--vault <caminho>]

Opções:
  --ttl <duracao>   Tempo de vida da sessão antes de auto-destruir (ex: 15m, 1h, 30s)
  --only <filtro>   Injeta apenas variáveis de credenciais que correspondam ao filtro
  --vault <caminho> Arquivo de cofre alternativo`)
		return
	}

	var ttl time.Duration
	filterEntry := ""
	customVault := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--ttl" && i+1 < len(args) {
			d, err := time.ParseDuration(args[i+1])
			if err == nil {
				ttl = d
			}
			i++
		} else if arg == "--only" && i+1 < len(args) {
			filterEntry = args[i+1]
			i++
		} else if arg == "--vault" && i+1 < len(args) {
			customVault = args[i+1]
			i++
		}
	}

	vaultPath := resolveVaultPath(customVault)
	code := runner.RunShell(ttl, filterEntry, vaultPath)
	os.Exit(code)
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

func resolveVaultPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}

	// Modo portátil: se existir vault.enc na pasta do executável ou atual
	if _, err := os.Stat("vault.enc"); err == nil {
		abs, errAbs := filepath.Abs("vault.enc")
		if errAbs == nil {
			return abs
		}
		return "vault.enc"
	}

	defaultPath, err := config.GetDefaultVaultPath()
	if err == nil {
		return defaultPath
	}

	return "vault.enc"
}

func tryAutoUpdateOnBoot() {
	endpoint := config.GetCloudEndpoint()
	updated, err := updater.AutoUpdate(endpoint, true)
	if err == nil && updated {
		fmt.Println("🚀 O Kofre foi atualizado para a versão mais recente! Reiniciando...")
		os.Exit(0)
	}
}

func handleUpdate() {
	endpoint := config.GetCloudEndpoint()
	fmt.Printf("🔍 Verificando atualizações em %s...\n", endpoint)
	updated, err := updater.AutoUpdate(endpoint, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao atualizar: %v\n", err)
		os.Exit(1)
	}
	if !updated {
		fmt.Println("Nenhuma atualização necessária.")
	}
}

func handlePro() {
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	endpoint := config.GetCloudEndpoint()

	fmt.Println("🚀 Ativando Kofre Cloud Pro (Modo Piloto)...")
	provisionURL := fmt.Sprintf("%s/v1/auth/provision", strings.TrimRight(endpoint, "/"))

	resp, err := http.Post(provisionURL, "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao conectar à API: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Falha na ativação (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var res struct {
		Token   string `json:"token"`
		Plan    string `json:"plan"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao processar resposta: %v\n", err)
		os.Exit(1)
	}

	cfg.Mode = "kofre_cloud"
	cfg.CloudEnabled = true
	cfg.KofreToken = res.Token
	cfg.CloudEndpoint = endpoint

	if err := config.SaveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao salvar configuração: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Licença Kofre Cloud Pro ativada com sucesso!")
	fmt.Printf("  • Token:     %s\n", res.Token)
	fmt.Printf("  • Endpoint:  %s\n", endpoint)
	fmt.Println()

	// Sincroniza o cofre local existente se houver
	vaultPath := resolveVaultPath("")
	if data, err := os.ReadFile(vaultPath); err == nil && len(data) >= 60 {
		fmt.Println("Sincronizando cofre local com a nuvem...")
		cloudStore := storage.NewKofreCloudStorage(endpoint, res.Token)
		ctx := context.Background()
		if err := cloudStore.Save(ctx, data); err == nil {
			fmt.Println("✓ Cofre sincronizado com a nuvem em segurança!")
		} else {
			fmt.Printf("⚠️  Aviso: falha na sincronização inicial: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("Dica: Use 'kofre telegram' para vincular alertas e desbloqueio remoto com timeout.")
}

func handleTelegramLink() {
	cfg, _ := config.LoadConfig()
	if cfg == nil || !cfg.CloudEnabled || cfg.KofreToken == "" {
		fmt.Fprintln(os.Stderr, "Erro: você precisa estar conectado ao Kofre Cloud para vincular o Telegram.")
		fmt.Fprintln(os.Stderr, "Execute primeiro: kofre login <seu-token>")
		os.Exit(1)
	}

	endpoint := config.GetCloudEndpoint()
	url := fmt.Sprintf("%s/v1/telegram/link-request", strings.TrimRight(endpoint, "/"))

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.KofreToken))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Falha na conexão com a nuvem: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Erro da API (%d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var res struct {
		Code    string `json:"code"`
		Bot     string `json:"bot"`
		LinkURL string `json:"link_url"`
		Expires string `json:"expires"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao processar resposta: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  Kofre 🔐 Vinculação de Segurança com Telegram             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Para receber alertas em tempo real e ativar o Botão de Pânico:")
	fmt.Println()
	fmt.Println("1. Abra o link direto no seu Telegram:")
	fmt.Printf("   👉 %s\n\n", res.LinkURL)
	fmt.Println("2. Ou envie o código abaixo diretamente para o bot @" + res.Bot + ":")
	fmt.Printf("   Código: %s  (Válido por %s)\n\n", res.Code, res.Expires)
	fmt.Println("Após tocar em 'Iniciar' ou enviar o código, seu Telegram estará conectado.")
}
