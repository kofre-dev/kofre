# KOFRE003: chave envolvida e detalhes separados

## Decisão de engenharia

- **Problema:** o formato anterior construía um JSON com todas as credenciais antes de cifrar, e notas/anexos podiam permanecer em strings/slices em texto simples.
- **Evidência:** `marshalVault`, decoder padrão de notas/anexos e cópias em `cloneEntry`; o código anterior do vault usava diretamente a chave derivada pelo Argon2id para cifrar esse JSON completo.
- **Invariante:** o conteúdo persistido permanece autenticado; senha errada, adulteração, truncamento e troca de contexto falham antes da publicação do cofre. Fechar o cofre revoga os handles entregues à interface.
- **Causa:** ausência de separação entre a chave derivada da senha e a chave dos dados, serialização global e acesso irrestrito a notas/anexos por campos públicos.
- **Referência:** AES-256-GCM com dados associados; Argon2id permanece com 64 MiB, três passagens e paralelismo quatro. Não foi introduzido algoritmo criptográfico próprio.
- **Solução:** DEK aleatória de 32 bytes protegida pela KEK derivada da senha; catálogo de cifras autenticado; cifra autenticada própria por item e para a identidade de conta; notas/anexos selados durante a sessão.
- **Previsão:** um catálogo decifrado não contém senhas, notas ou conteúdo de anexos; abertura e gravação processam um item por vez; os detalhes não ficam em texto simples durante o repouso da interface.
- **Riscos/não objetivos:** não protege uma sessão contra execução arbitrária no processo, administrador/kernel, captura durante uso ou adivinhação offline de uma senha fraca. Duas camadas de cifra não aumentam a entropia da senha. Não é abertura totalmente lazy: itens são processados individualmente ao carregar e seus campos sensíveis são imediatamente selados.
- **Oráculo:** fixtures independentes de formato legado; round-trip com identidade, notas e anexos; chave errada; adulteração de framing/catálogo/detalhe; troca de senha; varredura de memória de subprocesso fictício com controle positivo; fuzz de envelopes.
- **Gate:** nenhum conteúdo perdido na migração, nenhuma adulteração aceita, zero ocorrência dos canários no processo ocioso com leitura completa, controle positivo detectável, testes de consumidores aprovados. A camada de storage preserva backup antes da primeira gravação migrada.

## Arquivo

| Intervalo em bytes | Conteúdo |
| --- | --- |
| 0–7 | `KOFRE003` |
| 8–23 | Salt de 16 bytes do Argon2id |
| 24–39 | Identificador aleatório do cofre, 16 bytes |
| 40–99 | DEK envolvida pela KEK: nonce de 12 bytes, chave cifrada de 32 bytes e tag de 16 bytes |
| 100 em diante | Catálogo cifrado pela DEK: nonce de 12 bytes, ciphertext variável e tag de 16 bytes |

O cabeçalho de 40 bytes é autenticado como parte do AAD. O contexto é:

```text
cabeçalho || 0x00 || propósito UTF-8 || 0x00 || ID do item UTF-8
```

Os propósitos fixos são `chave`, `catalogo`, `item` e `conta`. Somente `item` tem ID final não vazio. Isso separa os usos e impede reaproveitar uma cifra de item em outro identificador, outra conta ou outro cofre sem uma autenticação válida. Nonces são gerados por `crypto/rand` em cada operação; nenhuma cifra reutiliza deliberadamente o mesmo nonce.

O catálogo é JSON com metadados do vault, uma lista `{id, ciphertext}`, identidade de conta cifrada em propósito separado e o backup pendente de conta (que já é ciphertext). Nenhum desses valores contém detalhes decifrados dos itens. O detalhe individual preserva o schema JSON de exportação em um objeto com apenas aquela entrada; seu buffer é apagado depois de cifrar/decifrar.

## Chaves, abertura e troca de senha

1. Argon2id deriva a KEK da senha e do salt.
2. A KEK decifra a DEK, autenticando cabeçalho e propósito.
3. A DEK decifra o catálogo.
4. Cada item é autenticado e decifrado separadamente; senhas, notas e anexos são selados na memória antes de avançar ao próximo item.
5. A DEK fica em `SealedBuffer`, revogada ao fechar o vault. O chamador mantém a KEK segundo o mecanismo já existente de sessão.

A troca explícita de senha chama `RotacionarChaveDados`: gera outra DEK e outro identificador de cofre antes de fechar a chave anterior. Os itens recebem novas cifras, e o novo envelope usa a nova KEK/salt. Assim, a DEK extraída de um backup anterior não abre as versões gravadas depois da troca. Salvamentos comuns preservam a DEK e renovam os nonces. Uma cópia antiga do arquivo continua dependendo da senha antiga: mudar a senha não revoga backups ou cópias já obtidas por terceiros.

No Windows, os buffers selados usam `CryptProtectMemory` com escopo do processo. Nas outras plataformas, a selagem atual usa uma chave que também permanece no processo; isso reduz texto simples ocioso, sem isolamento contra um atacante que leia toda a memória.

## API de detalhes na memória

Entradas gerenciadas não disponibilizam notas em `SecretEntry.Notes` nem bytes de anexos em `Attachment.Data`. Esses campos continuam servindo como entrada para importadores. Leitores usam:

```go
entry.WithNotes(func(value []byte) error { /* usar sem reter */; return nil })
attachment.WithData(func(value []byte) error { /* usar sem reter */; return nil })
```

Os callbacks recebem buffers temporários apagados ao retornar. `HasNotes`/`HasData` informam disponibilidade sem abrir o conteúdo. `SetNotes` cria um handle temporário independente para edição, incluindo a remoção de uma nota; depois de `AddEntry`/`UpdateEntry`, o chamador usa `CloseNotes`. O fechamento do cofre revoga notas/anexos dos clones já entregues.

Busca textual em notas continua disponível e abre uma nota por vez em buffers apagáveis. A exportação explícita devolve texto simples, como exigido pelo consumidor, que deve apagá-lo. Renderização, editor, clipboard e processos filhos são fronteiras adicionais e têm seus próprios controles; esta mudança não garante eliminar todas as cópias criadas fora do vault.

## Compatibilidade

O leitor continua aceitando `MYCOFRE1`, `KOFRE001` e `KOFRE002`. Toda nova gravação usa `KOFRE003`; clientes antigos rejeitam o formato em vez de descartar informações silenciosamente. `UnpackHeader` devolve o envelope completo para v3, permitindo a autenticação do framing, e mantém o payload AES original para os formatos antigos. `DecryptAndLoad` escolhe o decoder correspondente.

Nenhuma migração deve sobrescrever a cópia anterior sem backup persistido. O Cloud deve aceitar `KOFRE003` como blob cifrado; ele não recebe a KEK nem a DEK e não participa da decifração.
