# Kofre 🔐

> **O cofre de senhas e credenciais Zero-Knowledge desenvolvido para o terminal.**  
> Rápido, seguro, escrito em Go e blindado por criptografia moderna.

[![Go Version](https://img.shields.io/github/go-mod/go-version/kofre-dev/kofre?style=flat-square)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-emerald.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-blue?style=flat-square)](#instalação)
[![Zero-Knowledge](https://img.shields.io/badge/Crypto-Argon2id%20%2B%20AES--256--GCM-green?style=flat-square)](#segurança)

---

## ⚡ Instalação Rápida

### Windows (PowerShell)
```powershell
irm https://kofre.dev/install.ps1 | iex
```

### Linux / macOS (Bash)
```bash
curl -fsSL https://kofre.dev/install.sh | bash
```

### Via Go
```bash
go install github.com/kofre-dev/kofre/cmd/kofre@latest
```

---

## 🚀 Como Usar

### 1. Interface Visual Interativa (TUI)
Abra o cofre com navegação em tela cheia pelo teclado ou mouse:
```bash
kofre
```

**Atalhos da TUI:**
- `↑ / ↓` ou `j / k`: Navega pelas credenciais com rolagem suave.
- `Enter`: Copia a senha para o clipboard (auto-destrói da memória em 45s).
- `c`: Copia o nome de usuário / e-mail.
- `u`: Abre o site / URL diretamente no navegador.
- `/`: Filtro em tempo real por título, usuário ou nota.
- `d`: Deleta credencial com confirmação e sync em background.
- `q`: Fecha e limpa a memória com segurança.

### 2. Injeção de Segredos em Memória (Sem arquivos `.env`)
```bash
# Executa um comando injetando os segredos como variáveis de ambiente
kofre exec -- npm run build

# Abre um subterminal temporário com as credenciais carregadas na RAM
kofre shell
```

### 3. Importador Inteligente
Migre do seu navegador ou gerenciador anterior em segundos:
```bash
# Google Chrome ou Microsoft Edge (CSV padrão)
kofre import senhas_chrome.csv

# Bitwarden ou 1Password (JSON ou CSV)
kofre import export_bitwarden.json

# Chaves privadas SSH ou Certificados Digitais A1
kofre import ~/.ssh/id_ed25519.pem
kofre import certificado_a1.pfx
```

---

## 🛡️ Arquitetura de Segurança (Zero-Knowledge)

- **Argon2id (RFC 9106):** Derivação de chave à prova de GPUs com 64 MB de memória e salt de 128-bit gerado criptograficamente.
- **AES-256-GCM:** Criptografia autenticada de ponta a ponta com Nonce aleatório de 12 bytes por gravação.
- **Sincronização Segura:** O servidor em nuvem (S3/Kofre Cloud) recebe apenas bytes criptografados. Nem a nuvem, nem provedores de hospedagem têm acesso às suas credenciais.
- **2FA Gatekeeper via Telegram:** Autorização e auditoria em tempo real pelo bot oficial [@kofredev_bot](https://t.me/kofredev_bot).

---

## 📖 Documentação Completa

Para a documentação técnica detalhada, consulte a pasta [`docs/`](docs/) ou acesse [https://kofre.dev/docs](https://kofre.dev/docs).

---

## 📄 Licença

Distribuído sob a licença **MIT**. Consulte [`LICENSE`](LICENSE) para mais informações.
