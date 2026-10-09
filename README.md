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

### Download Direto dos Binários Compilados
Você também pode baixar os executáveis prontos diretamente na aba de **[GitHub Releases](https://github.com/kofre-dev/kofre/releases)**:
- `kofre-windows-amd64.exe` (Windows x64)
- `kofre-linux-amd64` / `kofre-linux-arm64` (Linux)
- `kofre-darwin-amd64` / `kofre-darwin-arm64` (macOS Intel / Apple Silicon)

### Via Go Toolchain
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
# Injeta somente API_TOKEN da credencial com título exato "API produção"
kofre exec --entry "API produção" --fields API_TOKEN -- npm run build

# Abre um subterminal temporário com as credenciais carregadas na RAM
kofre shell --entry "AWS" --fields AWS_ACCESS_KEY_ID,AWS_SECRET_ACCESS_KEY --ttl 15m
```

A seleção da credencial e dos campos é obrigatória. Use o ID quando houver
títulos repetidos; `--only` permanece como alias de `--entry`, com correspondência
exata. Os nomes dos campos viram variáveis em maiúsculas e sublinhados. Notas,
anexos e outros campos não são incluídos. `KOFRE_PIN` e `MYCOFRE_PIN` nunca são
repassadas ao filho. O programa iniciado e seus descendentes podem ler, copiar
ou persistir os valores selecionados; o TTL encerra somente o shell iniciado.

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

## Conexões de bancos de dados

No cadastro de credenciais, escolha **Banco de dados** com `←/→` na categoria.
Salve tipo, host, porta, nome do banco, usuário, senha e configuração TLS.
Para SQLite, informe o caminho do arquivo. Use `Ctrl+G` no campo de senha
para gerar uma senha forte e `Ctrl+S` para salvar.

As conexões ficam no cofre cifrado e podem ser compartilhadas explicitamente
com sua organização, pelas permissões existentes. O Kofre guarda a configuração;
não conecta ao servidor de banco. [Detalhes do cadastro e proteção](docs/BANCOS_DE_DADOS.md).

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
