package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// RunInteractiveConfig guia o usuario em um assistente simples no terminal
func RunInteractiveConfig() error {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = DefaultConfig()
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║               Kofre 🔐 Assistente de Configuração             ║")
	fmt.Println("║                   (https://kofre.dev)                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Println("Escolha o modo de armazenamento:")
	fmt.Println("  [1] Local (Offline no computador — 100% Gratuito)")
	fmt.Println("  [2] Kofre Cloud (SaaS gerenciado — https://kofre.dev)")
	fmt.Println("  [3] Meu Próprio Bucket S3 / Cloudflare R2 (Self-Hosted)")
	fmt.Print("\nOpção [1, 2 ou 3] (padrão 1): ")

	opt, _ := reader.ReadString('\n')
	opt = strings.TrimSpace(opt)

	switch opt {
	case "2":
		cfg.Mode = "kofre_cloud"
		cfg.CloudEnabled = true
		fmt.Print("\nDigite seu Token do Kofre Cloud (ex: kfr_live_...): ")
		token, _ := reader.ReadString('\n')
		cfg.KofreToken = strings.TrimSpace(token)
		plano, err := ConsultarPlanoCloud(GetCloudEndpoint(), cfg.KofreToken)
		if err != nil {
			return err
		}
		cfg.ContaID, cfg.PlanoCloud = plano.ContaID, plano.Plano
		fmt.Println("✓ Kofre Cloud configurado!")

	case "3":
		cfg.Mode = "custom_s3"
		cfg.CloudEnabled = true

		fmt.Print("\nNome do Bucket S3: ")
		bucket, _ := reader.ReadString('\n')
		cfg.S3Bucket = strings.TrimSpace(bucket)

		fmt.Print("Região (padrão: us-east-1): ")
		reg, _ := reader.ReadString('\n')
		reg = strings.TrimSpace(reg)
		if reg == "" {
			reg = "us-east-1"
		}
		cfg.S3Region = reg

		fmt.Print("Endpoint customizado (vazio para AWS oficial, ou URL do Cloudflare R2): ")
		ep, _ := reader.ReadString('\n')
		cfg.S3Endpoint = strings.TrimSpace(ep)

		fmt.Print("AWS / R2 Access Key ID: ")
		ak, _ := reader.ReadString('\n')
		cfg.S3AccessKey = strings.TrimSpace(ak)

		fmt.Print("AWS / R2 Secret Access Key: ")
		sk, _ := reader.ReadString('\n')
		cfg.S3SecretKey = strings.TrimSpace(sk)

		fmt.Println("✓ Bucket S3 configurado!")

	default:
		cfg.Mode = "local"
		cfg.CloudEnabled = false
		fmt.Println("✓ Modo Local selecionado.")
	}

	fmt.Println("\nCanal de Notificação / Liberação de Acesso:")
	fmt.Println("  [1] Telegram (Recomendado)")
	fmt.Println("  [2] Discord Webhook")
	fmt.Println("  [3] Apenas Senha Local (Sem mensagens)")
	fmt.Print("\nOpção [1, 2 ou 3] (padrão 1): ")

	notifOpt, _ := reader.ReadString('\n')
	notifOpt = strings.TrimSpace(notifOpt)

	if notifOpt == "1" {
		cfg.TelegramAuth = true
		fmt.Print("\nTelegram Bot Token: ")
		botToken, _ := reader.ReadString('\n')
		if strings.TrimSpace(botToken) != "" {
			cfg.TelegramBot = strings.TrimSpace(botToken)
		}

		fmt.Print("Seu Chat ID do Telegram: ")
		chatID, _ := reader.ReadString('\n')
		if strings.TrimSpace(chatID) != "" {
			cfg.TelegramChat = strings.TrimSpace(chatID)
		}
		fmt.Println("✓ Telegram configurado!")
	} else {
		cfg.TelegramAuth = false
	}

	if err := SaveConfig(cfg); err != nil {
		return fmt.Errorf("falha ao salvar configurações: %w", err)
	}

	cfgPath, _ := GetDefaultDir()
	fmt.Printf("\n✓ Configurações salvas com sucesso em:\n  %s\\config.json\n\n", cfgPath)
	return nil
}
