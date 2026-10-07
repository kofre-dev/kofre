# Atualizações autenticadas — versão 1.0.20

## Ficha de engenharia

**Problema:** o servidor fornecia o executável e seu SHA-256; comprometer a
publicação permitia trocar os dois. **Evidência:** `/v1/publish` calculava e
armazenava os metadados no Cloud e o cliente confiava nesse JSON.
**Invariante:** a instalação por atualização só promove bytes autorizados pela
chave de publicação, independente de quem controla Railway ou o bucket.
**Hipótese causal:** assinatura assimétrica, com raiz pública no cliente e
privada fora da infraestrutura de distribuição, impede fabricar uma release
aceita apenas adulterando servidor, hash ou tamanho.
**Referência:** [Ed25519 da biblioteca padrão Go](https://pkg.go.dev/crypto/ed25519);
separação entre assinatura e distribuição, com limites de rollback/freeze
comparados à [especificação TUF](https://theupdateframework.github.io/specification/latest/).
Este protocolo não implementa o framework TUF completo.
**Solução:** descriptor assinado por plataforma, manifesto limitado, comparação
estrita de versão, validade e SHA-256/tamanho verificados antes da promoção.
**Previsão:** adulteração/ausência de assinatura, plataforma trocada, versão
anterior e metadata excessiva serão recusadas sem substituir o executável.
**Riscos/não objetivos:** bootstrap inicial, cliente já adulterado, comprometimento
da máquina que assina, rollback do próprio executável pelo administrador e
negação de serviço permanecem fora dessa garantia.
**Oráculo/gate:** testes de corrupção de todos os campos, downgrade, expiração,
JSON excessivo e download adulterado; todos devem passar, inclusive com `-race`.

## Contrato

`releases/latest.json` preserva `version`, `release_date`, `notes`, `platforms` e
os campos legados `version`, `url`, `sha256`, `size` de cada plataforma. A partir
de 1.0.20, cada plataforma acrescenta `platform`, `key_id`, `released_at`,
`expires_at`, `notes` e `signature` hexadecimal Ed25519.

A assinatura cobre um array JSON em ordem fixa:

```text
["kofre:release:v1", key_id, platform, version, url, sha256, size,
 released_at, expires_at, notes]
```

O caminho aceito é exatamente `/v1/download/{platform}?version={version}`.
Versões têm três componentes numéricos canônicos. Somente descritores com chave
compilada autorizada, SHA-256 de 32 bytes, binário entre 1 byte e 100 MiB e datas
válidas são aceitos. O JSON recebido não pode ultrapassar 64 KiB. Controle de
terminal nas notas é recusado, com exceção de tabulação e quebra de linha.

A versão, data e notas globais continuam por compatibilidade com clientes
antigos. Clientes novos usam exclusivamente os dados assinados da própria
plataforma. Assim a publicação incremental de Linux/macOS/Windows não faz um
cliente baixar uma versão diferente da anunciada para seu executável.

Não há fallback para release sem assinatura. Sem publicação autenticada o
atualizador automático preserva o executável e o uso local continua disponível;
a atualização pedida explicitamente mostra o erro. A comparação com a versão
compilada impede downgrade; replay de uma release assinada mais recente que a
instalada ainda é possível até expirar. A validade padrão é 90 dias, máxima 370
dias; publicação periódica ou renovação assinada é necessária. A verificação
depende do relógio do sistema e não impede congelamento dentro dessa janela.

## Chave e rotação

Raiz pública fixada em `pkg/releasesign/manifest.go`:

```text
key_id: kofre-release-2026-01
Ed25519: a08c941b86b17cbdf634c16ede1a07f7b307dc23e15262739a55d3c00de853b8
```

O Cloud tem somente a chave pública. Não existe configuração de ambiente que
substitua a raiz do cliente. `release-sign`, no repositório privado do Cloud,
assina localmente um manifesto; o `publisher` envia apenas manifesto e binário.
O CI entrega artifacts para conferência, sem chave privada e sem publicação
automática de executáveis como release oficial.

Para rotação planejada, lançar primeiro um cliente assinado pela chave atual
que confie também na próxima raiz; depois assinar com a nova e aposentar a
antiga em versão posterior. Clientes que nunca receberam a ponte podem precisar
de reinstalação verificada. Se a raiz for comprometida antes dessa migração,
não há recuperação segura confiando somente em mensagens assinadas por ela.

## Limites da primeira instalação e clientes anteriores

Os instaladores PowerShell/shell mantêm tamanho e hash e só são gerados pelo
Cloud honesto a partir de manifesto autenticado. Isso impede que somente
adulterar o bucket produza um script de instalação válido no servidor honesto.

Porém, um script baixado e executado diretamente de um Cloud comprometido pode
remover qualquer checagem. A primeira instalação por `irm | iex` ou `curl | sh`
continua confiando no canal de distribuição. Não se deve anunciar esse fluxo
como resistente ao comprometimento do servidor. Uma verificação independente
exige obter a raiz/verificador por um canal confiável antes de executar o
binário; assinatura de código do sistema operacional é uma etapa distinta.

Clientes anteriores a 1.0.20 ignoram campos adicionais e continuam conseguindo
baixar a nova versão. A transição ainda depende das garantias antigas; a
assinatura obrigatória começa depois de instalar um cliente que a verifica.

## Validação desta implementação

Testes usam chaves Ed25519 efêmeras/determinísticas exclusivas de fixtures,
servidores HTTP e armazenamento S3 simulados. Há teste que executa o bloco real
do instalador PowerShell contra download fictício, sem iniciar ou registrar o
executável de fixture. Nenhum vault, credencial de produção ou release pública
foi utilizado nos testes. Não houve deploy ou publicação nesta alteração.
