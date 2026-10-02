# Relatório de segurança: chave persistida e exposição em memória

Data da constatação: 02/10/2026.

Este documento preserva o primeiro teste, pelo DPAPI, e acrescenta os retestes após a mudança da instalação. O caminho inicial ficou bloqueado pela ausência de `device_key.bin`; posteriormente, uma senha exclusivamente fictícia foi recuperada da memória com o cofre desbloqueado e confirmada pelo proprietário. Os estados e os limites de cada teste estão separados abaixo.

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

## Reteste após a remoção da chave persistida

O executável instalado mudou para o SHA-256 `a26e3606e20f614f74bd38634e89a0befce99736041324d8009fed5fc8416931`. O histórico do repositório passou a conter mudanças de remoção de chave persistida e endurecimento de memória. Foram consultados metadados e títulos dos commits, não os fontes nem seus diffs. O hash identifica o arquivo instalado observado; não é uma atestação de correspondência entre binário e commit.

Com o aplicativo fechado, `device_key.bin` não estava mais presente. O vault tinha o mesmo hash do primeiro teste. Assim, o procedimento anterior parou na recuperação da chave e não abriu o vault. A inspeção da configuração não encontrou campos explicitamente nomeados como material de chave. Isso não exclui outros mecanismos de recuperação ou persistência.

Depois, o proprietário iniciou o aplicativo e informou que ele permanecia bloqueado. O processo observado foi `Kofre.exe`, PID 29648. Foi possível obter um handle de leitura e examinar 20.111.360 bytes em 49 regiões privadas comprometidas e legíveis. Não foram encontrados os marcadores procurados do usuário/URL do teste original, em UTF-8 ou UTF-16LE, nem o prefixo JSON específico pesquisado. Ausência desses marcadores não prova ausência de qualquer segredo em outros formatos ou regiões.

Após o proprietário desbloquear o cofre, uma segunda leitura cobriu 97.947.648 bytes em 73 regiões elegíveis, sem falhas de leitura. O usuário e a URL originais foram encontrados, mas nenhuma senha real foi extraída nessa etapa. O token do processo leitor foi consultado com `GetTokenInformation(TokenElevation)` e retornou não elevado. Não houve pedido ou ativação de privilégios administrativos/debug.

## Segundo achado: senha de teste legível na memória

### Preparação e confirmação

Foi solicitado um registro fictício para evitar a exposição de contas reais. O proprietário criou e salvou a entrada `testedememoria@email.com` e confirmou que o cofre continuava desbloqueado.

A busca inicial, somente pelo e-mail, encontrou a string e uma referência compatível com o campo `Usuario`, mas não recuperou a senha nem estabeleceu a ligação entre os campos. Em seguida, o proprietário forneceu esta dica:

```text
Seis caracteres, começando com a e terminando com F: a****F
```

Com essa informação, a busca recuperou o candidato **`adminF`** em memória. O proprietário confirmou explicitamente que esse era o valor cadastrado para o teste.

**`adminF` é um valor exclusivamente fictício deste experimento, não uma recomendação de senha. A recuperação dependeu da dica de comprimento, início e fim. Não foi uma extração independente sem conhecimento prévio.** Nenhuma outra senha foi apresentada.

### Como a leitura foi feita

Um processo Python separado utilizou `ctypes` e funções de `kernel32.dll`:

1. `OpenProcess(0x0410, FALSE, pid)`: `PROCESS_QUERY_INFORMATION` (`0x0400`) combinado com `PROCESS_VM_READ` (`0x0010`). O Windows concedeu o handle.
2. `QueryFullProcessImageNameW`: conferência de que o PID correspondia ao executável instalado do Kofre antes da leitura.
3. `VirtualQueryEx`: enumeração das regiões do processo.
4. `ReadProcessMemory`: cópia dos bytes das regiões elegíveis para buffers do leitor, em blocos de até 1 MiB.

O filtro de regiões foi:

```text
State == MEM_COMMIT (0x1000)
Type == MEM_PRIVATE (0x20000)
PAGE_GUARD (0x100) ausente
(Protect & 0xff) em {0x02, 0x04, 0x08, 0x20, 0x40, 0x80}
```

O orçamento máximo foi de 128 MiB; a leitura final precisou de aproximadamente 26,1 MiB. Regiões de imagem e mapeamentos que não satisfaziam esse filtro não fizeram parte da varredura. Leituras curtas pontuais seguiram ponteiros para conferir nomes de campos, inclusive se esses nomes estivessem fora das regiões privadas varridas.

Não houve suspensão do processo, injeção de código, alteração de memória, tentativa de autenticação, leitura de clipboard ou criação de dump em disco. A ferramenta manteve cópias temporárias dos blocos na memória do próprio leitor; não se afirma que todas as cópias internas do Python tenham sido zeradas.

### Busca pelo padrão e validação das referências

Foi identificado no executável instalado o marcador de build Go. Isso orientou uma hipótese de representação em memória de strings por ponteiro e comprimento, em palavras de 64 bits little-endian. Não foram usados os fontes, símbolos de depuração ou uma declaração formal do tipo do aplicativo; a interpretação abaixo é uma heurística validada pelos conteúdos observados, não uma ABI universal de Go.

O procedimento foi:

1. Procurar nos blocos lidos o padrão de bytes `a[\x21-\x7e]{4}F`, que representa seis caracteres ASCII imprimíveis sem espaço, compatíveis com a dica. Também foi feita uma contagem do padrão equivalente em UTF-16LE.
2. Guardar em memória os endereços dos resultados ASCII, sem imprimir os valores gerais encontrados.
3. Procurar pares de palavras de 64 bits no formato `(ponteiro_para_resultado, 6)`, compatíveis com cabeçalhos de string de comprimento seis.
4. Para cada referência, ler 40 bytes a partir de 16 bytes antes dela. Interpretar os primeiros 33 bytes como quatro palavras de 64 bits e um byte: ponteiro do nome, comprimento do nome, ponteiro do valor, comprimento do valor e indicador booleano.
5. Conferir que o nome apontado era `Senha` ou `password`, o comprimento do valor era seis e o indicador era `1`, compatível com `protected: true`.
6. Reler os seis bytes do valor no processo e verificar igualdade com o candidato da varredura, reduzindo a possibilidade de usar somente uma cópia antiga do leitor.
7. Agrupar os valores distintos que passaram essas verificações. Somente o candidato único foi apresentado ao proprietário para confirmação.

Representação interpretada no ponto da referência:

```text
ref - 16 : ponteiro para o nome do campo
ref -  8 : comprimento do nome
ref +  0 : ponteiro para o valor
ref +  8 : comprimento do valor (6)
ref + 16 : indicador observado (1)
```

Um endereço bruto da senha não é incluído: muda entre execuções e não é necessário para reproduzir o método num ambiente de teste próprio.

### Evidência da leitura final

| Medida | Resultado |
|---|---:|
| PID observado | 29648 |
| Bytes privados elegíveis e lidos | 27.369.472 |
| Falhas de leitura dos blocos | 0 |
| Ocorrências ASCII do padrão | 41 |
| Valores ASCII distintos compatíveis com o padrão | 3 |
| Ocorrências do padrão em UTF-16LE | 2 |
| Referências compatíveis com strings de seis bytes | 28 |
| Referências também compatíveis com campo `Senha` protegido | 28 |
| Valores distintos após o filtro de campo | 1 |
| Candidato único | `adminF` |
| Confirmação do proprietário | Sim |

As 28 referências não representam 28 senhas diferentes nem comprovam 28 cópias distintas do conteúdo. Diferentes cabeçalhos podem apontar para os mesmos bytes. Também não foi determinado quais objetos ainda estavam ativos, quais eram temporários ou quais eram resíduos aguardando reutilização de memória.

**A associação estrutural completa entre o objeto da entrada do e-mail fictício e o campo recuperado não foi demonstrada.** A confirmação de que o candidato era a senha cadastrada veio do proprietário. Isso é evidência de recuperação do valor de teste, mas não deve ser apresentado como reconstrução completa do registro ou extração de todas as credenciais.

Depois da criação da entrada fictícia, o hash do vault passou para `0e5acc03405f980bc969f40b43c26fcd2f289ec492017a3fa3159e1c3ca9e2c7` e permaneceu igual durante as leituras. A mudança anterior à leitura corresponde ao cenário em que o usuário havia salvado uma nova entrada; o agente não gravou no vault. `device_key.bin` continuou ausente.

## Por que essa recuperação é possível

Há duas condições observadas simultaneamente:

- Os bytes da senha fictícia existiam em texto legível no espaço de memória do Kofre.
- O Windows permitiu ao processo leitor obter `PROCESS_VM_READ` e copiar esses bytes.

A criptografia do arquivo protege sua representação persistida. Ela não cifra automaticamente as cópias que o aplicativo produz ao carregar, editar ou apresentar os dados. Neste teste, a senha foi recuperada dessas cópias em memória, sem descriptografar novamente o arquivo e sem obter a chave mestra.

O indicador `protected` é um dado interpretado pelo aplicativo. Mesmo que ele controle mascaramento na interface, isso não constitui uma permissão de memória aplicada pelo Windows. Foi observada sua presença junto de um valor legível; sua semântica exata na implementação ainda precisa ser verificada pelo responsável pelos fontes.

`VirtualLock` mantém páginas residentes e impede sua escrita no arquivo de paginação enquanto bloqueadas. Essa API não estabelece isolamento contra outro processo que tenha um handle com direito de leitura. Não foi medido se as páginas específicas da senha estavam travadas; a conclusão relevante é que `ReadProcessMemory` conseguiu lê-las.

Em Go, strings são imutáveis pela semântica da linguagem. Apagar um buffer mutável original não comprova que strings, objetos ou buffers derivados também desapareceram. O fluxo exato que produziu as referências observadas não foi localizado nos fontes; atribuí-lo a JSON, renderização da TUI, histórico de edição ou cópia de structs sem investigação seria uma hipótese, não uma causa comprovada.

### Referências técnicas deste segundo teste

- [Direitos de acesso a processos](https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights).
- [ReadProcessMemory](https://learn.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-readprocessmemory).
- [VirtualLock](https://learn.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-virtuallock).
- [Strings na especificação de Go](https://go.dev/ref/spec#String_types).

## Investigação e validação pendentes para memória

O responsável pela implementação deve rastrear a vida útil do campo secreto, desde criação/carregamento até edição, renderização, serialização, cancelamento e bloqueio. Conferir se segredos que não estão sendo usados permanecem em strings comuns, se buffers protegidos são convertidos em strings e se o caminho de limpeza cobre cópias temporárias e objetos antigos.

Possíveis medidas devem ser avaliadas por experimento: manter campos cifrados quando ociosos, reduzir o tempo de existência de texto legível, usar buffers mutáveis com propriedade e ciclo de vida claros, evitar cópias desnecessárias e separar acesso a segredos do restante da UI. Criptografar um campo mantendo sua chave igualmente acessível no mesmo processo não garante resistência a um leitor arbitrário da memória. Endurecimento de acesso ao processo pode reduzir exposição, mas deve ser validado para a identidade e os privilégios do cenário, sem promessa de proteção absoluta contra administrador ou comprometimento do sistema operacional.

**Ainda não foi executado o teste de bloqueio após esse desbloqueio com a senha fictícia.** O próximo experimento deve:

1. Usar um valor fictício novo por rodada, com baseline antes de criar/desbloquear, para evitar confundir resíduos de testes anteriores.
2. Medir separadamente após salvar, visualizar, ocultar e bloquear o cofre, mantendo o mesmo processo aberto.
3. Repetir após intervalos definidos e após reinício, informando exatamente quais regiões foram examinadas.
4. Comparar presença do valor conhecido, cópias/referências e eventual possibilidade de recuperação da chave. Ausência de uma string específica não prova ausência de segredos cifrados ou em outra representação.
5. Registrar testes com e sem dica separadamente. Neste experimento, a tentativa independente não recuperou a senha; a busca dirigida recuperou o valor confirmado.

Nenhuma correção de código foi aplicada pelo agente deste teste. A recuperação ocorreu com o cofre desbloqueado; não prova bypass de autenticação ou persistência da senha após o bloqueio.

## Correção nos fontes e validação isolada — 2026-10-02

As seções anteriores descrevem a auditoria do executável instalado e permanecem
como histórico. Nesta etapa houve autorização para ler e corrigir os fontes.
O executável instalado, o vault pessoal e a configuração do usuário não foram
substituídos. Os testes abaixo usam dados fictícios.

### Causa confirmada e decisão

**Problema:** campos `protected` permaneciam legíveis com o cofre aberto.
**Evidência:** `Field.Value` era uma `string`; `ManagedVault` conservava o JSON
decodificado inteiro, e `Entries`, `GetEntry` e `Search` compartilhavam slices
dos campos com a TUI. O formulário continuava referenciado após salvar/cancelar.
`cleanup` apagava a chave, mas não encerrava os dados do cofre.
**Invariante:** campos protegidos ociosos não conservam seu valor em
`Field.Value`; encerrar o cofre revoga os handles já distribuídos; arquivos
`KOFRE001` e `MYCOFRE1` continuam usando o esquema JSON/AES-GCM existente.
**Solução:** buffers cifrados com tempo de vida explícito, acesso temporário por
callback, serialização controlada e limpeza do estado de interface.
**Previsão:** leitura de memória da fixture não encontra o canário após carregar,
empacotar ou fechar; um controle positivo deliberadamente exposto é encontrado.
**Riscos/não objetivos:** isso não cria isolamento contra execução de código no
processo, administrador, kernel comprometido ou captura durante uso explícito.
**Oráculo/gate:** testes de compatibilidade e ciclo de vida, varredura externa com
controle positivo, `go test -race ./...`, `go vet ./...` e build Windows.

### Implementação

- `pkg/crypto/sealed*.go`: no Windows, `CryptProtectMemory`/
  `CryptUnprotectMemory` com `CRYPTPROTECTMEMORY_SAME_PROCESS`. O conteúdo cifrado
  permanece entre usos; a cópia temporária decifrada é sobrescrita ao terminar
  o callback, inclusive quando ele retorna erro. `Close` é idempotente.
- `pkg/vault/protected.go`: carregamento de valores protegidos por buffers JSON
  mutáveis, sem produzir as antigas strings permanentes. `WithValue` entrega
  bytes temporários; a busca/listagem recebe valores protegidos vazios e handles
  cifrados. `Pack` evita colocar senhas nos buffers reutilizados de `json.Marshal`
  e apaga seu JSON temporário, inclusive buffers anteriores ao crescer.
- `pkg/vault/vault.go`: cópias independentes dos slices públicos, revogação ao
  editar/excluir/fechar e recusa de gravação após fechamento. Erros ao proteger
  entradas e exportar variáveis agora são retornados aos chamadores.
- `pkg/tui/secure_input.go`: senha mestre e campo de senha usam vetores de runas
  controlados, sobrescritos antes de realocação ou descarte. O caminho normal
  de criação/salvamento recebe bytes, sem converter a senha em `string`.
  A geração de senha também fornece um buffer apagável.
- `pkg/tui/app.go`: chave de sessão cifrada quando ociosa; bloqueio por
  `Ctrl+L` ou cinco minutos sem teclado/mouse; revelação limitada a 15 segundos;
  tentativa de limpeza do clipboard após 30 segundos e ao bloquear/sair,
  preservando conteúdo diferente que outro aplicativo tenha colocado nele.
  Salvar/cancelar limpa o formulário. Editar deixa a senha vazia para conservar
  a existente e preserva os campos adicionais. Erros de gravação são mostrados;
  repetir um salvamento não duplica a nova entrada.
- No desbloqueio, os atalhos de plano e Telegram passaram a `Ctrl+P` e `Ctrl+T`:
  as letras `p` e `t` precisam poder fazer parte da senha mestre.
- `cmd/kofre/main.go`, importação e runner encerram os dados de sessão após uso.
  O comentário de `SetErrorMode` foi corrigido: ele não impede leitura de memória.

### Resultados reproduzíveis

Executado no Windows/amd64 com Go 1.26.0:

```powershell
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go test ./pkg/vault -run TestProtectedVaultMemoryIsolation -v -count=1 -timeout=60s
go test ./pkg/vault -run '^$' -fuzz=FuzzDecodeValueCompatibility -fuzztime=5s -parallel=2
go build -o build/kofre-seguranca.exe ./cmd/kofre
```

O teste de memória gera um canário aleatório de 48 caracteres em cada execução,
cria um arquivo cifrado temporário e inicia **somente seu próprio subprocesso**.
O filho recebe o caminho da fixture, nunca o canário em argumentos/ambiente.
O pai usa `OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ)`,
`VirtualQueryEx` e `ReadProcessMemory`, percorrendo as regiões privadas
comprometidas e legíveis, sem páginas de guarda. Procura UTF-8, UTF-16LE e
UTF-32LE (`[]rune`), sem salvar dumps e sem solicitar elevação.

Última execução do teste, incluindo criação e atualização de um campo protegido:

| Etapa | Bytes lidos | Falhas de leitura | Ocorrências do canário |
| --- | ---: | ---: | ---: |
| Após carregar | 15.390.976 | 0 | 0 |
| Após criar | 15.407.360 | 0 | 0 |
| Após atualizar | 15.407.360 | 0 | 0 |
| Após empacotar | 16.144.640 | 0 | 0 |
| Após `Close` | 16.144.640 | 0 | 0 |
| Controle positivo com JSON legível vivo | 16.144.640 | 0 | 1 |

Os testes de unidade cobrem valor com espaços, caracteres de controle, Unicode,
valores grandes, leitura por um decoder independente do formato anterior,
fixture JSON legada, revogação de handles, exclusão, edição sem perder campos,
cancelamento, repetição após falha de armazenamento, limpeza da senha mestre,
inatividade e expiração da revelação. O clipboard é simulado nos testes de TUI:
verifica cópia, expiração, preservação de conteúdo externo e erro de escrita.
O teste de processo é real para o núcleo do cofre e as APIs Windows; não é uma
validação de todas as telas do terminal nem do executável pessoal instalado.

O fuzzing diferencial comparou o decoder dos buffers com `encoding/json`.
Encontrou uma diferença de aceitação de espaço vertical fora da string; ela foi
corrigida e o caso ficou em `pkg/vault/testdata/fuzz` como regressão. A execução
seguinte passou com **289.661 avaliações em cinco segundos**. A rodada final de
`go test -race ./... -count=1`, `go vet ./...` e build Windows passou.

Artefato de teste: `build/kofre-seguranca.exe`.
SHA-256: `ff78e8c99ddd8aa025949af77ee43c21ecd7e9b3337641c5190d0af12f6c5495`.
Esse arquivo não foi instalado sobre o Kofre em uso.

### Limites e pendências explícitas

1. **Revelar/copiar/exportar continua sendo exposição deliberada.** Renderização
   do terminal, biblioteca de clipboard e ambiente de processos usam strings
   e/ou cópias fora dos buffers controlados. Ocultar a tela ou limpar o clipboard
   não comprova apagamento de todas essas cópias. Histórico/sincronização do
   clipboard e memória do terminal/processo filho não são apagados por esta
   implementação. Falha do sistema ao limpar o clipboard também é possível.
2. A proteção em repouso na RAM aplica-se a **campos `Protected` e à chave de
   sessão da TUI**. Metadados, campos não protegidos, notas e anexos mantêm sua
   representação anterior. Dados confidenciais nesses locais exigem extensão
   da representação protegida e novos testes. Importadores legados que recebem
   strings também podem deixar cópias transitórias fora do controle do cofre.
3. A varredura procura um valor conhecido em representações específicas.
   Não prova ausência de outras representações, schedules internos de AES,
   resíduos no runtime, paginação, dumps ou possibilidade de reconstruir chaves.
   O bloqueio automatizado foi validado no núcleo e no modelo da TUI; o reteste
   manual do aplicativo instalado, após revelar/copiar e bloquear, continua
   pendente até executar a versão nova em um ambiente de teste.
4. Não foi implantado broker com outra identidade, TPM ou isolamento por
   processo separado. Não foi aplicada uma DACL apresentada como barreira
   absoluta contra o próprio usuário/administrador. `VirtualLock` e
   `SetErrorMode` não são substitutos dessas fronteiras de segurança.
5. Fora do Windows, o buffer usa AES-GCM e chave efêmera no mesmo processo,
   o que evita texto simples ocioso mas não isola a chave de um leitor de RAM.
   Builds completos de Linux/macOS continuam bloqueados por erros já presentes
   no código anterior: `config.go` referencia `EncryptWithDPAPI`/
   `DecryptWithDPAPI` inexistentes fora do Windows, e `mem_unix.go` referencia
   `unix.Prctl`/`PR_SET_DUMPABLE` indisponíveis no build Darwin. Não houve mudança
   silenciosa de política de proteção dos tokens para contornar esses erros.

Referência da proteção usada:
[CryptProtectMemory — Microsoft](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectmemory).
Ela exige decifrar antes de usar e recomenda apagar o texto legível após o uso;
não promete tornar o processo inviolável.

## Publicação — versão 1.0.7, Windows x64

Em 2026-10-02, publicação autorizada pelo usuário após o smoke local. Inclui as
correções de memória acima, rodapé responsivo e ajuda F1. `CurrentVersion` foi
atualizada de 1.0.6 para 1.0.7; a versão pública anterior era 1.0.5.

- Artefato: `build/kofre-1.0.7-windows-amd64.exe`, compilado com `-trimpath -ldflags="-s -w"`.
- Tamanho: 8.393.216 bytes.
- SHA-256: `b23fa1def62b00eaf29445dd1e48090cb1a7bf48811b2a1fbd16af4d919e6ea3`.
- Publicação pelo `cmd/publisher` do projeto `kofre-cloud`, endpoint `/v1/publish`.
- Download: `https://kofre.dev/v1/download/windows-amd64`.
- Metadados: `https://kofre.dev/v1/version/latest`.
- Landing page com aba Windows (CMD) implantada no Railway, serviço `kofre-cloud`,
  ambiente `production`, deployment `045c0c68-cc7b-4e9e-ba09-4c51c62022a2`.

Validação: testes com detector de concorrência do cliente passaram; servidor
passou em `go test ./...`, `go vet ./...` e build Linux. A resposta pública de
metadados e o download foram conferidos contra versão, tamanho e SHA-256 locais.
O binário público anterior e seus metadados foram preservados em `kofre-cloud/build`
para reversão. Linux/macOS não foram publicados nesta rodada.
