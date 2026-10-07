# Reforços do cliente — candidato 1.0.20

Decisão em 07/10/2026. Escopo: senha e tentativas na interface, exposição em
memória e em subprocessos, integridade de sincronização e atualizações.
Somente dados fictícios são usados na validação. O cofre pessoal não é um fixture.

## Registro de engenharia

- Problema/evidência: o formato anterior decifrava um JSON contendo todos os
  segredos; notas e anexos permaneciam em claro; subprocessos herdavam
  `KOFRE_PIN`; downloads podiam substituir a cópia local antes da autenticação
  GCM; o hash do executável vinha da mesma origem do executável.
- Invariantes: nenhuma perda da última cópia validada; leitura dos formatos
  anteriores; senha nunca salva; segredos gerenciados revogados ao bloquear;
  nenhuma atualização sem assinatura verificável por chave pública compilada.
- Hipótese: diminuir a quantidade e a duração de texto claro reduz a exposição
  incidental; autenticação antes da promoção e assinatura independente removem
  as respectivas falhas de integridade. Nenhuma delas aumenta a entropia da senha.
- Solução: chave de dados aleatória envelopada pela chave derivada com Argon2id;
  formato KOFRE003 com detalhes autenticados por item; buffers selados para
  notas/anexos; seleção explícita de campos para subprocessos; staging remoto;
  assinatura Ed25519 por plataforma. A chave privada de publicação fica fora
  dos repositórios e do Cloud, protegida pelo usuário do sistema operacional.
- Previsão: abertura/salvamento do formato novo não produzem um JSON em claro
  com todos os segredos; adulterações não promovem conteúdo nem executáveis.
- Riscos/não objetivos: seis caracteres com composição continua sendo uma
  senha possivelmente previsível. Bloqueios da interface não resistem à cópia
  do arquivo nem ao controle do sistema. Proteção de memória não promete
  segurança contra administrador, kernel ou leitura durante uso autorizado.
- Referências: RFC 9106 (Argon2id); AES-GCM da biblioteca padrão Go;
  modelo de metadados assinados e prevenção de rollback do TUF; documentação
  CryptProtectMemory/DPAPI da Microsoft. Não é um protocolo criptográfico próprio.
- Oráculo/gate: roundtrip legado/novo, troca de senha, corrupção e adulteração,
  revogação de handles, exposição de memória em fixtures Windows, seleção de
  ambiente do filho, promoção remota, assinatura/expiração/rollback e testes
  dos fluxos da TUI. Só gerar candidato após testes e revisão de integração.

## Política aprovada

Novas senhas e trocas: mínimo de **6 caracteres no total**, com ao menos uma
letra, símbolo ou espaço interno. Somente números são recusados. Letras sem
números são aceitas; espaços externos não contam. Abertura de cofres anteriores
não aplica retroativamente essa regra. Não se trata de estimativa de resistência
à força bruta: `123@456` atende à composição, mas continua previsível.

Após cada três tentativas sem sucesso na interface: 5, 15, 30, 60, 120 e até
300 segundos de espera. O estado não contém senha, chave ou hash da senha.
Persistência local e exclusão mútua entre instâncias reduzem reinícios e
paralelismo pelo aplicativo. Quem controla os arquivos pode apagar ou restaurar
esse estado; ele não é uma barreira contra ataques offline.

O CLI que abre cofres locais usa o mesmo controle de tentativas. `exec` e
`shell` passam a exigir `--entry` e `--fields`, selecionados explicitamente.
Variáveis `KOFRE_PIN`/`MYCOFRE_PIN` não são repassadas ao processo filho; o uso
deliberado de senha em variável de ambiente ainda a expõe ao ambiente original.

Notas não são renderizadas em claro na tela de detalhes nem no formulário.
Podem ser selecionadas e copiadas ou reveladas por confirmação explícita.
A revelação dura dez segundos e retorna automaticamente, sem deixar leitores
de teclado concorrentes. O buffer controlado é apagado antes dessa espera;
o terminal ainda precisa exibir o texto. Controles de terminal são escapados
nessa exibição, enquanto a cópia mantém o conteúdo exato.

Falha transitória na limpeza do clipboard é repetida até três tentativas,
inclusive ao encerrar. Conteúdo copiado por outro aplicativo é preservado;
falha persistente avisa para limpeza manual.

## Operação e limites

KOFRE003 passa a ser escrito nas gravações novas; cofres anteriores continuam
legíveis e recebem backup antes da primeira migração. Clientes antigos não
entendem o formato novo e precisam ser atualizados antes de usar essa cópia.
Backups antigos mantêm a senha antiga e devem ter o mesmo cuidado do cofre.

A assinatura protege o atualizador que já possui a chave pública confiável.
O primeiro instalador obtido do site ainda depende da integridade desse canal.
O backup portátil da chave de publicação exige senha própria e uma cópia
guardada pelo responsável; copiar somente o arquivo DPAPI não permite recuperar
a chave em outro perfil do Windows.

## Validação

Em 07/10/2026 passaram `go test -race ./...`, `go vet ./...` e
`git diff --check` nos repositórios cliente e Cloud. A revisão independente
também encontrou e corrigiu:

- comparação/gravação local sem exclusão entre instâncias; a promoção validada
  agora usa CAS, backup e escrita sob o mesmo lock de escritores cooperantes;
- ausência de revisão persistida após login/recuperação, que causava conflito
  falso na primeira edição depois de reabrir;
- envio aceito pelo Cloud seguido de falha no ACK local: a retomada só confirma
  quando uma releitura devolve exatamente os bytes enviados;
- prazo de espera que podia se deslocar indefinidamente após relógio recuado;
  o limite de cinco minutos é persistido antes de retornar o bloqueio.

Scanner Windows independente, com subprocesso criado pelo teste: seis estados
ociosos (abertura legada, criação, edição, gravação, abertura v3, fechamento),
15.395.072 a 16.148.736 bytes lidos por estado, sem falhas de leitura e **zero
ocorrências dos canários**. Controle positivo: três ocorrências detectadas em
16.156.928 bytes lidos. Inclui senha, nota e anexo/base64. Não tenta recuperar
chaves nem prova resistência contra leitura arbitrária da RAM. O fuzz curto
do envelope executou somente dez casos: não equivale a um pentest abrangente.

Gerados builds `windows-amd64`, `linux-amd64`, `linux-arm64`, `darwin-amd64` e
`darwin-arm64` em `dist/1.0.20-seguranca`, cada um com manifesto Ed25519 local
conferido contra seus bytes. Linux/macOS tiveram validação de compilação;
execução nativa validada somente no Windows. Smoke do executável Windows
`--version` retornou `Kofre v1.0.20 (windows-amd64)`, com APPDATA/LOCALAPPDATA
isolados em diretório temporário.

`kofre.exe` e `kofre-preview.exe` locais receberam o build Windows.
SHA-256: `0833ee6c5b6faea1cd4a6f0e87483135ec31e6cc185feebe6356b7a14f114ebd`.

Não houve implantação no Railway, publicação pública, substituição da
instalação em Programs ou pentest no cofre do usuário nesta etapa. A raiz
privada de publicação foi criada fora do Git em diretório com ACL restrita,
protegida por DPAPI. O backup externo portátil da raiz ainda precisa ser
guardado pelo responsável usando o procedimento em
[`RELEASES_ASSINADAS.md`](../../kofre-cloud/docs/RELEASES_ASSINADAS.md).
