# Kofre 🔐 Guia de Formatos e Exemplos de Importação

Este documento detalha os formatos recomendados para importar credenciais, senhas, tokens de API, chaves SSH e certificados digitais para o Kofre de forma limpa, segura e organizada.

---

## 🚀 Como Executar a Importação

No terminal, execute o comando apontando para o arquivo ou diretório desejado:

```powershell
# Pré-visualização com confirmação interativa [S/n]
kofre import caminho\do\arquivo.csv

# Importação automática sem prompt de confirmação
kofre import caminho\do\arquivo.json --yes

# Importação direta de chaves ou certificados individuais
kofre import C:\chaves\id_ed25519.pem
kofre import C:\certificados\empresa_a1.pfx
```

---

## 1. Formato CSV (Recomendado para Planilhas e Exportações)

O formato CSV é ideal para importar grandes volumes de credenciais a partir do Excel, Google Sheets, Bitwarden, 1Password ou Chrome.

* **Separadores suportados:** Vírgula (`,`), ponto-e-vírgula (`;`) ou Tabulação (`\t`). Detectado automaticamente.
* **Colunas reconhecidas (independente de maiúsculas/minúsculas):**
  * `Title` / `Titulo` / `Nome` / `Servico`: Título de identificação da credencial.
  * `Category` / `Categoria` / `Tipo`: `password`, `token`, `ssh_key`, `certificate`, `auth`, `note`.
  * `Username` / `Usuario` / `Login` / `Email`: Nome de usuário, login ou e-mail.
  * `Password` / `Senha` / `Pass` / `Secret` / `Token`: Senha ou segredo (sempre protegido e mascarado com `••••••`).
  * `URL` / `Site` / `Link` / `Host`: Endereço do serviço ou host de conexão.
  * `Notes` / `Notas` / `Obs`: Observações adicionais.
  * *Colunas extras:* Qualquer outra coluna será automaticamente cadastrada como campo personalizado.

### Exemplo `senhas.csv`:
```csv
Title,Category,Username,Password,URL,Notes
GitHub Deploy,token,dev_deploy,ghp_exemploTokenSecreto12345,https://github.com,Token de deploy CI/CD
AWS Producao,password,admin_root,SenhaForte#2026,https://aws.amazon.com,Conta principal da empresa
Banco Inter PJ,password,financeiro@empresa.com,MinhaSenha!99,,Chave PIX e conta corrente
Servidor Linux Bastion,ssh_key,ubuntu,"-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdzc2gtcn
...
-----END OPENSSH PRIVATE KEY-----",54.232.10.50:22,Chave Ed25519 do servidor
Certificado e-CNPJ A1,certificate,contato@empresa.com,SenhaDoPFX123,,Certificado Serasa válido até 2027
```

### 🌐 Como Exportar e Importar do Google Chrome e Microsoft Edge

O Kofre possui suporte nativo ao padrão de exportação dos navegadores baseados em Chromium (**Google Chrome, Microsoft Edge, Brave e Opera**): `name,url,username,password,note`.

#### No Google Chrome:
1. Acesse o Gerenciador de Senhas digitando na barra de endereços: `chrome://password-manager/settings`
2. Na seção **Exportar senhas**, clique em **Fazer download do arquivo**.
3. Confirme o PIN ou senha do Windows e salve o arquivo (ex: `senhas_chrome.csv`).
4. No terminal, execute:
   ```powershell
   kofre import senhas_chrome.csv
   ```

#### No Microsoft Edge:
1. Acesse a Carteira de Senhas digitando na barra de endereços: `edge://wallet/passwords`
2. Clique no menu de três pontos (`...`) no canto superior e selecione **Exportar senhas**.
3. Confirme a autenticação do Windows e salve o arquivo (ex: `senhas_edge.csv`).
4. No terminal, execute:
   ```powershell
   kofre import senhas_edge.csv
   ```

> [!TIP]
> **Tratamento inteligente de URLs:** Quando o Chrome ou Edge exporta um registro sem nome (`name` vazio), o Kofre extrai automaticamente o domínio da `url` (por exemplo, `app.slack.com` ou `github.com`) como título da credencial, evitando itens sem identificação. Após a importação, exclua o arquivo CSV desprotegido do seu disco.


---

## 2. Formato JSON (Ideal para Integrações e Chaves Multilinhas)

O formato JSON é altamente legível e seguro para chaves que possuem quebras de linha (`\n`), como chaves privadas SSH e certificados PEM.

### Exemplo `senhas.json`:
```json
[
  {
    "title": "GitHub Deploy",
    "category": "token",
    "username": "solivansoft",
    "password": "ghp_exemploTokenSecreto12345",
    "url": "https://github.com",
    "notes": "Token de deploy CI/CD"
  },
  {
    "title": "AWS Producao",
    "category": "password",
    "username": "admin_root",
    "password": "SenhaForte#2026",
    "url": "https://aws.amazon.com",
    "notes": "Conta principal"
  },
  {
    "title": "Servidor Linux Bastion",
    "category": "ssh_key",
    "username": "ubuntu",
    "password": "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ...\n-----END OPENSSH PRIVATE KEY-----",
    "url": "54.232.10.50:22",
    "notes": "Chave Ed25519 do cluster"
  },
  {
    "title": "Certificado SSL Wildcard",
    "category": "certificate",
    "username": "*.empresa.com",
    "password": "SenhaDoCertificadoAqui",
    "url": "https://api.empresa.com",
    "notes": "Renovação anual necessária em Outubro"
  }
]
```

---

## 3. Formato TXT Estruturado em Blocos

Se preferir manter suas credenciais em arquivo de texto plano (`.txt` ou `.md`), utilize separadores de bloco claros (`=== Titulo ===`, `### Titulo` ou `[Titulo]`):

### Exemplo `senhas.txt`:
```txt
=== GitHub Deploy ===
Categoria: token
Usuario: solivansoft
Senha: ghp_exemploTokenSecreto12345
URL: https://github.com
Notas: Token de deploy CI/CD

=== AWS Producao ===
Categoria: password
Usuario: admin_root
Senha: SenhaForte#2026
URL: https://aws.amazon.com

=== Servidor Linux Bastion ===
Categoria: ssh_key
Usuario: ubuntu
URL: 54.232.10.50:22
Notas: Chave Ed25519 do cluster
Senha: -----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdzc2gtcn
...
-----END OPENSSH PRIVATE KEY-----

=== Certificado e-CNPJ A1 ===
Categoria: certificate
Usuario: financeiro@empresa.com
Senha: SenhaDoCertificado123
Notas: Certificado A1 Serasa emitido em 2026
```

---

## 4. Importação Direta de Arquivos Criptográficos

Para chaves e certificados que já estão salvos em seus próprios arquivos, o Kofre oferece importação direta sem necessidade de montar CSV ou JSON:

| Tipo | Extensões | Como o Kofre Trata |
| :--- | :--- | :--- |
| **Chaves SSH** | `.pem`, `.key`, `.ppk` | Extrai o payload criptográfico, protege com AES-256 e categoriza como `SSH`. |
| **Certificados A1 / P12** | `.pfx`, `.p12` | Armazena o binário original como anexo criptografado e categoriza como `Certificados`. |
| **Prints e Cartões 2FA** | `.png`, `.jpg`, `.jpeg` | Armazena a imagem como anexo criptografado e categoriza como `Auth/2FA`. |

Exemplo de comando:
```powershell
kofre import C:\Users\usuario\Desktop\minha_chave.pem
kofre import C:\Users\usuario\Desktop\certificado.pfx
```

---

## 🛡️ Regras de Segurança do Importador

1. **Limite de Tamanho:** Nenhum arquivo com mais de **10 MB** é processado, prevenindo exaustão de memória e ataques de negação de serviço (DoS).
2. **Arquivos Ignorados:** Códigos-fonte (`.php`, `.cs`, `.vb`, `.sql`, `.js`, `.py`, etc.) e binários executáveis (`.exe`, `.dll`, `.bin`, `.iso`) são sumariamente descartados para evitar poluição do cofre.
3. **Higienização de Strings:** Todos os campos passam por remoção de `NULL bytes` (`\x00`), caracteres de controle e validação de UTF-8.
4. **Campos Protegidos:** Qualquer campo cujo nome ou valor indique dados sensíveis (`senha`, `password`, `token`, `secret`, `key`, `chave`, `pin`) é marcado como `Protected: true`, sendo mascarado na tela e descriptografado apenas em memória RAM sob demanda.
