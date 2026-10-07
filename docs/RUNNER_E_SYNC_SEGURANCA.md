# Seleção de segredos e validação da sincronização

Data: 07/10/2026. Escopo: cliente, processos filhos e promoção de bytes remotos.

Problema: `exec`/`shell` exportavam todas as credenciais quando o filtro estava
vazio e herdavam as variáveis de desbloqueio; o sincronizador substituía a cópia
local antes de verificar a autenticação criptográfica do arquivo baixado.
Evidência: `ToEnvMap("")`, herança de `os.Environ()` e gravação em `Load`.
Invariante: o filho recebe somente campos explicitamente selecionados e nunca
`KOFRE_PIN`/`MYCOFRE_PIN`; um candidato remoto não substitui a base local nem
avança a revisão antes de passar por `UnpackHeader` e `DecryptAndLoad`.

Solução: `--entry` identifica uma entrada por ID ou título exato único;
`--fields` seleciona seus campos exatos. Filtro parcial, seleção vazia,
ambiguidades, nomes reservados, colisões e valores com byte nulo são recusados.
As variáveis de desbloqueio são retiradas do ambiente herdado, inclusive com
caixa diferente. A senha digitada usa buffers mutáveis; valores escolhidos são
copiados em buffers temporários e convertidos para strings somente na fronteira
exigida por `os/exec`. O cofre é fechado antes da execução e as referências ao
ambiente são soltas após `Start`.

`SyncStorage.Load` mantém bytes remotos apenas como candidato em memória.
Após decifrar e validar, o chamador executa `storage.ConfirmarCarga`. A operação
confere que o arquivo local não mudou, preserva backup, instala e só então
confirma a revisão. Senha errada, senha alterada em outro PC e tag inválida não
descartam a cópia anterior. Sem cópia local, a primeira baixa também aguarda
validação. `Save` recusa alterações enquanto houver candidato não confirmado.
Falha no ACK conserva o conteúdo já autenticado e permite repetir a confirmação.
CAS e pendências anteriores continuam ativos. O comando `pull` autentica antes
de instalar e recusa descartar um envio local pendente.

`LocalStorage` preserva um backup cifrado na primeira gravação que converte
KOFRE001/MYCOFRE1/KOFRE002 para KOFRE003. Gravações posteriores em KOFRE003 não
geram backups de migração repetidos.

Previsão/oráculo: subprocesso real recebe um campo fictício selecionado e nenhum
PIN ou campo adjacente; fixtures com Argon2id/AES reais confirmam preservação
local/revisão sob senha incorreta e adulteração, instalação após autenticação,
backup de migração único e recusa de substituição após alteração concorrente.
Gate: testes de runner, storage e CLI, `go vet` e revisão do diff.

Limites: variáveis entregues ao filho são acessíveis a ele e podem ser copiadas
ou persistidas. A API de processos exige strings imutáveis; não prometemos sua
eliminação física da RAM. O TTL encerra o shell iniciado, não revoga dados já
copiados nem garante encerrar todos os descendentes. Os controles locais de
tentativas não impedem um programa alternativo de atacar uma cópia do vault.
A confirmação compara, preserva backup e grava sob um lock de arquivo
(`LockFileEx`/`flock`) compartilhado por todos os escritores `LocalStorage`.
`SubstituirSeIgual` recusa a instalação se outro processo cooperante mudou a
base durante a validação. Escritores externos que ignoram o lock continuam
fora dessa garantia. A espera pelo lock respeita o cancelamento do contexto.
