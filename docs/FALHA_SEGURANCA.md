# Exposição do vault pelo acesso à chave DPAPI na sessão Windows

Data da constatação: 02/10/2026.

## Resultado e alcance

Foi possível descriptografar o vault local com o Kofre fechado, sem consultar os fontes, sem fornecer senha mestra e sem interagir com a interface do aplicativo. A operação ocorreu na sessão Windows do proprietário, com autorização dele para testar sua instalação.

O conteúdo foi autenticado com sucesso pelo AES-GCM e interpretado como JSON válido. Foram encontradas 1.075 entradas. A entrada específica indicada pelo proprietário foi localizada pelo usuário e pela URL; seu campo `Senha`, marcado com `protected: true`, estava disponível como string de 12 caracteres após a descriptografia.

Não foram expostos valores de senhas, chaves ou tokens nos resultados apresentados. Não houve tentativa de login no serviço externo, acesso à API de nuvem, modificação do vault, alteração do executável ou dump da memória do Kofre.

**Falha demonstrada:** no modo local testado, um processo com acesso aos arquivos do Kofre e capacidade de usar o DPAPI na sessão Windows atual consegue obter a chave suficiente para abrir o vault, mesmo sem o aplicativo em execução.

**Não foi demonstrado:** quebra de AES-GCM, ataque de força bruta à senha mestra, acesso por outro usuário Windows, descriptografia em outro computador com apenas uma cópia do vault ou comprometimento da conta externa. Tampouco foi comprovado que esse comportamento ocorre em todos os modos do produto.

## Condições observadas

- Executável instalado em `%LOCALAPPDATA%/Programs/Kofre/Kofre.exe`.
- Na identificação inicial, o processo estava em execução; antes da tentativa de descriptografia, a consulta de processos já não encontrou Kofre/Cofre. Não foi encerrado pelo agente.
- Dados encontrados em `%APPDATA%/Kofre`.
- Configuração efetiva: `mode: "kofre_cloud"`, com `vault_path` apontando para o `vault.enc` encontrado. Isso identifica a configuração examinada; não houve teste do backend cloud.
- Arquivos acessíveis ao processo de teste: `config.json`, `device_key.bin` e `vault.enc`.
- As ACLs observadas permitiam acesso ao proprietário, SYSTEM, administradores e identidades locais adicionais, incluindo grupos usados pelo ambiente Codex. Não foi estabelecido se essas permissões adicionais são padrão do produto ou efeito do ambiente de testes.

## Evidência dos artefatos examinados

Hashes identificam os arquivos no momento do teste; não contêm conteúdo secreto.

| Artefato | Tamanho | SHA-256 |
|---|---:|---|
| Kofre.exe | 8.327.680 bytes | `7bda0e340375ea8400d98ea987479514bef650d007964be09800f70de1156202` |
| config.json | 843 bytes | `44fe12f8098220c1ca4cd6b4c502d185dbeb725a5ddb75d8215fd60dac91f871` |
| device_key.bin | 262 bytes | `1d07900f0161a034bca06d4080ed526ae4d82ba7d7197961cb511eeeb9454856` |
| vault.enc | 474.396 bytes | `91adf26e575de9725080498f7f3357a773c1ec833946b8db6a7eaf0b6bd7a47c` |

O hash do vault foi reconferido após as operações e permaneceu igual. O executável não tinha assinatura Authenticode; isso não foi explorado e não explica a descriptografia.

## Procedimento exato realizado

### 1. Localização, sem leitura dos fontes

Foram consultados processos e metadados de instalação para localizar o executável. Depois, foram listados diretórios com nomes Kofre/Cofre em AppData e locais comuns de dados. A análise ficou restrita aos arquivos da instalação e aos dados locais; nenhum arquivo-fonte do projeto foi lido.

Foi lida a estrutura do JSON de configuração. Valores de tokens não foram impressos. O campo de modo e a correspondência do caminho do vault foram conferidos separadamente.

### 2. Reconhecimento do envelope DPAPI

`device_key.bin` começava com o cabeçalho binário característico de um blob DPAPI:

```text
01 00 00 00 d0 8c 9d df 01 15 d1 11 8c 7a 00 c0 4f c2 97 eb
```

Esse cabeçalho não é a chave. Foi usado apenas para identificar o mecanismo de proteção.

### 3. Recuperação da chave de dispositivo

Um processo Python, usando `ctypes`, carregou `crypt32.dll` e chamou a API Windows `CryptUnprotectData` diretamente, passando:

- `pDataIn`: `DATA_BLOB` com os 262 bytes de `device_key.bin`;
- `ppszDataDescr`: `NULL`;
- `pOptionalEntropy`: `NULL`;
- `pvReserved`: `NULL`;
- `pPromptStruct`: `NULL`;
- `dwFlags`: `1`, isto é, `CRYPTPROTECT_UI_FORBIDDEN`;
- `pDataOut`: um `DATA_BLOB` vazio para receber o resultado.

A chamada retornou sucesso e produziu **32 bytes**. Não houve prompt, solicitação de senha mestra ou chamada ao aplicativo. Esses bytes foram usados em memória como a chave do AES-GCM. O buffer retornado pelo Windows foi zerado antes de `LocalFree`; a cópia em `bytearray` foi zerada após o uso. Isso não comprova limpeza de todas as cópias internas criadas pelo Python ou pela biblioteca criptográfica. Nenhuma chave foi gravada em arquivo ou impressa.

### 4. Identificação do layout e abertura do vault

Sem documentação do formato ou leitura dos fontes, foram testados layouts delimitados, usando a mesma chave recuperada e validando a tag de autenticação. Não foi feita busca por chaves ou senhas.

Foram consideradas posições iniciais `0, 4, 8, 16, 20, 24, 32`, nonces de 12 ou 16 bytes e tag de 16 bytes no fim ou no começo do corpo. O primeiro layout autenticado apareceu na tentativa 21:

```text
vault[0:24]    = prefixo não interpretado neste teste
vault[24:36]   = nonce de 12 bytes
vault[36:-16] = ciphertext
vault[-16:]   = tag de autenticação de 16 bytes
```

A operação que funcionou, usando `cryptography.hazmat.primitives.ciphers.aead.AESGCM`, foi:

```text
AESGCM(chave_de_32_bytes).decrypt(
    nonce = vault[24:36],
    data = vault[36:],
    associated_data = None
)
```

`data` contém ciphertext e tag anexada, como exigido pela interface utilizada. O resultado tinha **474.344 bytes** e era JSON válido em UTF-8. Não foi determinado o significado dos primeiros 24 bytes. O sucesso com AAD `None` descreve a operação observada; não identifica, por si só, todos os mecanismos de integridade do formato.

### 5. Confirmação da exposição

O JSON tinha campos `schema_version`, `created_at`, `updated_at` e `entries`. A lista `entries` continha 1.075 registros.

Dentro do registro solicitado, `fields` era uma lista de objetos com `name`, `value` e `protected`. O campo de nome `Senha` tinha `protected: true`, mas seu `value` já era uma string legível. A flag não impediu a leitura por esse caminho. Não se presume, sem examinar a implementação, que ela deveria representar criptografia adicional; pode ser uma flag de apresentação.

Os comandos imprimiram apenas contagens, nomes de campos, comprimentos, flags e resultado de autenticação. O valor da senha e os demais segredos ficaram fora da saída.

## Brecha encontrada e mecanismo causal

A chave de dispositivo protegida pelo DPAPI é recuperável no contexto Windows do processo de teste. Essa chave é suficiente, sem segredo adicional fornecido pelo usuário, para descriptografar o vault examinado.

Portanto, o limite de proteção efetivamente demonstrado é o acesso ao contexto DPAPI e aos arquivos, não o bloqueio da interface ou a ausência do processo Kofre. Um processo nesse contexto pode repetir a operação do aplicativo por fora dele.

Isso é compatível com o funcionamento documentado do DPAPI: normalmente, dados podem ser recuperados com as mesmas credenciais de logon e no mesmo computador. O DPAPI não deve ser tratado como uma barreira exclusiva entre aplicativos executando no mesmo contexto de usuário. A proteção adicional da chave, caso necessária pelo requisito do produto, precisa ser explicitamente projetada.

Referências oficiais:

- [CryptUnprotectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptunprotectdata).
- [CryptProtectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata).

Não foi investigado se o blob usa escopo usuário ou máquina; o sucesso na sessão atual, sozinho, não distingue esses casos. Também não foi determinada a origem das permissões adicionais da pasta. Esses pontos precisam ser conferidos pelo agente responsável na implementação e na instalação.

## Direção de correção para o agente responsável

Primeiro, confirmar o requisito: o vault fechado deve resistir apenas à cópia offline por terceiros, ou também a outro processo executando como o usuário Windows? O teste reprova o segundo requisito no modo observado.

Se a proteção exige um segredo independente do logon Windows, a chave que abre o vault não pode permanecer integralmente recuperável apenas pelos arquivos locais e pelo DPAPI dessa sessão. Avaliar derivação de uma chave de proteção a partir de senha mestra com KDF apropriada, parâmetros explícitos e versionados, para proteger a chave de dados com criptografia autenticada. Conferir também recuperação, sincronização, troca de senha e migração de vaults existentes. Não alterar o formato sem validar esses caminhos.

Auditar especialmente desbloqueio automático e conveniências do modo cloud: se um caminho persistente recupera a mesma chave sem o fator independente, pode anular a proteção pretendida do estado bloqueado. Apenas esconder a UI, ofuscar a chave, restringir os nomes dos arquivos ou exigir que o aplicativo esteja aberto não resolve o mecanismo demonstrado.

Endurecer ACLs ajuda contra outras identidades, mas não cria, por si só, isolamento contra processos com a mesma identidade do usuário autorizado. Qualquer solução deve declarar esse limite. Com o cofre desbloqueado, um processo hostil suficientemente privilegiado pode ter acesso à memória, entrada ou saída; esse é outro cenário, ainda não testado aqui.

## Critérios de validação da correção

Reproduzir em um vault descartável, com credenciais fictícias, seguindo as mesmas condições: aplicativo fechado, nenhuma senha mestra fornecida ao processo de teste e mesmo usuário Windows.

1. Conferir a configuração efetiva e os arquivos de chave, inclusive backups e caches.
2. Repetir a recuperação via DPAPI. Recuperar um material auxiliar não deve ser suficiente para abrir o vault sem o fator independente, caso esse seja o requisito.
3. Demonstrar que o vault pode ser aberto normalmente pelo usuário com os fatores exigidos e que os dados anteriores foram preservados após a migração.
4. Validar senha incorreta, reinício, bloqueio automático, recuperação e sincronização sem persistir uma chave equivalente que contorne a correção.
5. Validar adulteração de ciphertext e tag, além dos campos relevantes do envelope conforme o contrato do formato.
6. Separar os resultados dos cenários: cópia offline, mesmo usuário Windows, outro usuário, administrador e cofre desbloqueado. Não chamar aprovação em um cenário de segurança universal.

O agente desta constatação não alterou código nem aplicou correção. Este relatório documenta a instalação examinada e o procedimento que funcionou; não afirma que a falha permanece após mudanças posteriores.
