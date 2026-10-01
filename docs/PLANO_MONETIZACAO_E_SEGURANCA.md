# Kofre 🔐 — Plano de Monetização Global & Simulação de Ataques

Este documento consolida a estratégia de expansão comercial no mercado global (meta: 1.000 usuários pagantes) e o protocolo de testes de segurança extrema (Threat Modeling & Simulação de Ataques) para o **Kofre** (`kofre.dev`).

---

## PARTE 1: ESTRATÉGIA DE MONETIZAÇÃO GLOBAL

### 1.1 Posicionamento de Mercado (Go-To-Market)
O Kofre não concorre com gerenciadores de senha convencionais (como Bitwarden, 1Password ou LastPass voltados para famílias). O Kofre é posicionado como uma ferramenta de infraestrutura e produtividade para desenvolvedores, sysadmins e DevOps.

* **Tagline Global:**
  > *"Zero-Knowledge secrets in memory, never on disk. Guarded by your Telegram."*
* **A Dor Resolvida:**
  Elimina o risco de vazamento de credenciais via arquivos `.env` commitados por engano no Git, sem a complexidade pesada de ferramentas corporativas (Doppler, HashiCorp Vault) e sem a lentidão do CLI de gerenciadores tradicionais.
* **Diferenciais Competitivos:**
  1. **Sub-20ms Startup:** Binário nativo em Go de arquivo único, sem dependência de Node.js, Python ou JVM.
  2. **In-Memory Injection:** Injeção direta de variáveis de ambiente no processo filho (`kofre exec -- python main.py`) ou subshell efêmero (`kofre shell --ttl 30m`), com limpeza de memória (`ZeroBytes`).
  3. **Guardião Telegram:** Autorização em tempo real no celular com timeout estrito de 3 minutos e comando remoto `/panic`.
  4. **Armazenamento Híbrido:** Modo Cloud nativo ou S3/R2 próprio do usuário (*Bring Your Own Storage*).

### 1.2 Estrutura de Preços (Pricing Tiers)

| Plano | Preço | Público | Recursos Principais |
|---|---|---|---|
| **Free (Local)** | $0 / R$ 0 | Todos os devs | Cofre AES-256 local ilimitado, `kofre exec`, `kofre shell`, CLI e TUI interativa. |
| **Cloud Pro (Mensal)** | $4 / mês | Devs e Freelancers | Sincronização automática S3 Zero-Knowledge, Bot Telegram com Timeout de 3 min, Botão de Pânico, múltiplos dispositivos. |
| **Cloud Pro (Anual)** | $39 / ano (~$3.25/mês) | Devs e Freelancers | Todos os recursos Pro com 20% de desconto. |
| **Lifetime Founder (Piloto)** | $59 único (Limitado) | Primeiros 100 usuários | Acesso vitalício para validar tração e gerar caixa inicial imediato. |

### 1.3 A Matemática dos 1.000 Usuários Pagantes
* **1.000 assinantes a $4/mês:** $4.000 MRR (~R$ 22.000/mês).
* **Custo de infraestrutura projetado (Railway + S3):** ~$25 a $40/mês (blobs criptografados leves de ~50 KB por usuário e scale-to-zero na API).
* **Margem operacional:** > 90%.

### 1.4 Plataforma de Checkout: Polar.sh (Merchant of Record)
Para cobrar internacionalmente sem a complexidade burocrática de impostos locais (VAT europeu, impostos americanos de consumo):
* **Polar.sh:** Atua como Merchant of Record (MoR) oficial construído sobre a infraestrutura do Stripe.
* **Métodos aceitos:** Cartão de crédito internacional, Apple Pay, Google Pay.
* **Fluxo no CLI:**
  1. Usuário roda `kofre pro` ou aperta `[p]` na TUI.
  2. A API gera uma sessão de checkout via API do Polar (`POST /v1/checkouts/custom`).
  3. O terminal exibe a URL curta e abre o navegador automaticamente (`https://buy.polar.sh/...`).
  4. Webhook do Polar notifica `POST /v1/billing/webhook`.
  5. API provisiona o token `kfr_live_...` e o terminal do usuário atualiza na hora via polling com backoff exponencial.

---

## PARTE 2: PROTOCOLO DE TESTES DE SEGURANÇA & THREAT MODELING

Para certificar a resiliência do Kofre antes da escala comercial, o sistema deve ser submetido à auditoria dos 5 vetores críticos de ataque:

```
                      ┌────────────────────────────────────────┐
                      │            VETORES DE ATAQUE           │
                      └────────────────────────────────────────┘
                                          │
         ┌─────────────────┬──────────────┴─────┬──────────────────┐
         ▼                 ▼                    ▼                  ▼
   [1. Invasão S3]   [2. MITM & Replay]   [3. RAM Scraping]  [4. Webhook & OTP]
```

### Vetor 1: Invasão da Infraestrutura e Vazamento do S3 (Zero-Knowledge)
* **Ameaça:** Comprometimento total das credenciais AWS ou acesso direto aos arquivos `vaults/<user_id>/vault.enc` no S3.
* **Controle Criptográfico:**
  * O servidor nunca processa chaves simétricas de decodificação.
  * O cofre é cifrado no cliente usando **AES-256-GCM** com chave de 256 bits derivada via **Argon2id** (64 MB de memória, 3 iterações, 4 threads, salt de 32 bytes criptograficamente aleatório).
  * A carga no S3 é estritamente: `Salt (32B) || Nonce (12B) || Tag (16B) || Ciphertext`.
* **Procedimento de Teste:**
  1. Extrair o arquivo bruto `vault.enc` direto do bucket.
  2. Executar teste de entropia de Shannon (deve atingir valor $\approx 8.000$, indicando ausência total de padrões legíveis).
  3. Submeter o arquivo a ataques com wordlists (rockyou) para atestar que o consumo de memória do Argon2id (64 MB/tentativa) inviabiliza brute-force paralelo massivo em GPU.

### Vetor 2: Interceptação de Rede (MITM) e Replay Attacks
* **Ameaça:** Um atacante intercepta a comunicação entre o CLI e a Railway, capturando o token Bearer `kfr_...` e tentando clonar o cofre.
* **Controle Criptográfico:**
  * O canal é estritamente TLS 1.3 com HSTS.
  * O token Bearer dá acesso apenas ao blob cifrado (que permanece indecifrável sem a senha do usuário).
  * **Alerta em Tempo Real:** Todo download do cofre dispara um alerta silencioso no Telegram do usuário com timestamp exato (`📥 Kofre Cloud: Download de cofre realizado às 15:04:05`).
  * **Revogação Instantânea:** Se o usuário não reconhecer o download, ele aciona `/panic` no bot, que marca `PanicLocked: true` e bloqueia instantaneamente qualquer nova requisição daquele token na API.

### Vetor 3: Extração de Segredos em Memória RAM (RAM Scraping)
* **Ameaça:** Presença de malware ou ferramenta de depuração (ProcDump, Cheat Engine, WinDbg) tentando ler o espaço de memória do processo `kofre.exe` enquanto desbloqueado.
* **Controle Criptográfico:**
  * Implementação da rotina de sanitização forçada de memória:
    ```go
    func ZeroBytes(b []byte) {
        for i := range b {
            b[i] = 0
        }
    }
    ```
  * Ao bloquear, ao encerrar ou ao receber sinais do SO (`SIGINT`, `SIGTERM`), as variáveis contendo a chave de 256 bits e os segredos em memória são imediatamente sobrescritas com zeros antes da liberação pelo Garbage Collector.
* **Procedimento de Teste:**
  1. Desbloquear o cofre na memória RAM.
  2. Disparar o fechamento do cofre.
  3. Gerar um dump completo do processo imediatamente após o fechamento e inspecionar os blocos de memória procurando por strings de credenciais conhecidas.

### Vetor 4: Forjamento de Webhooks e Brute-Force no Desafio Telegram
* **Ameaça:** Atacante tenta adivinhar o código OTP de 6 dígitos via requisições sequenciais no endpoint `/v1/auth/telegram-challenge/verify`.
* **Controle Criptográfico:**
  * **Janela Temporal Restrita:** Timeout inflexível de **3 minutos** (180 segundos). Após esse prazo, o desafio é destruído em memória.
  * **Rate Limiting:** Bloqueio após 3 tentativas inválidas consecutivas por IP/token.
  * **Hardware Binding (DPAPI):** O código OTP do Telegram apenas confirma a identidade do usuário. O desbloqueio automático no PC requer que a máquina possua a chave local protegida pela API de proteção de dados do Windows (`CryptProtectData`), impedindo que o atacante desbloqueie remotamente uma máquina não pareada fisicamente.

### Vetor 5: Bypasses Locais de Licença Pro
* **Ameaça:** Usuário edita o arquivo local `config.json` e insere um token fictício (`kfr_pro_fake123`) para tentar sincronizar gratuitamente no S3.
* **Controle Criptográfico:**
  * O gateway Railway valida o token contra o banco/tabela de licenças ativas na rota `POST /v1/auth/verify`.
  * Se o token não for reconhecido como emitido pelo Polar/Stripe/Asaas, o gateway retorna `HTTP 403 Forbidden` e rejeita qualquer operação de leitura ou escrita no S3.

---

## CRONOGRAMA DE EXECUÇÃO
1. **Fase 1 (Atual):** Piloto funcional com Railway, S3, Publisher isolado e Telegram Bot com Timeout.
2. **Fase 2:** Integração do Webhook oficial de faturamento (Polar.sh / Stripe) para geração automática de licenças de produção.
3. **Fase 3:** Criação da suíte automatizada de testes de segurança (Fuzzing da API e testes de integridade do cofre).
4. **Fase 4:** Lançamento público global no Hacker News (Show HN), Reddit (`r/golang`, `r/devops`) e Product Hunt.
