# Estudo: PIN curto, funcionamento offline e chaves de acesso

Data: 07/10/2026. Estado: proposta para discussão; não implementada nem aprovada como requisito do produto.

## Objetivo e conclusão

Investigar com três agentes — arquitetura, revisão adversarial e multiplataforma — como manter `vault.enc`, código aberto, uso offline e um PIN curto sem depender do TPM de cada computador.

Não encontramos uma construção que proteja contra a cópia completa do estado necessário à abertura quando esse estado é software copiável e o único segredo restante é `123456`. Quem possui as mesmas entradas pode executar o mesmo cálculo. Contadores locais, hash do executável, ID do hardware e dupla cifra não criam, por si só, uma autorização independente.

O candidato mais forte encontrado acrescenta uma concessão: **um token físico portátil USB/NFC com segredo próprio e verificação do PIN no token**. Dispensa o TPM do computador, mas exige outro hardware. Essa exigência ainda não foi aceita pelo usuário. A proposta usa primitivas existentes; não é uma descoberta criptográfica nem garante abertura exclusiva pelo executável oficial.

## Registro de engenharia

- **Problema:** permitir PIN curto e offline, resistindo ao roubo do arquivo e à cópia dos arquivos do computador.
- **Evidência:** implementação atual deriva chave com Argon2id; estudo demonstrou que outro executável da mesma conta Windows acessa uma amostra protegida por DPAPI.
- **Invariante:** nenhuma promessa pode depender de esconder o código, confiar em um `if` removível ou presumir que dados públicos são secretos.
- **Causa/hipótese:** uma fronteira de segurança independente precisa manter fora dos arquivos copiados um segredo necessário à abertura.
- **Referência:** CTAP/hmac-secret, documentação DPAPI e SDKs oficiais citados abaixo.
- **Solução candidata:** chave aleatória do vault encapsulada com resultado secreto do token; verificação nativa do usuário obrigatória.
- **Previsão:** copiar vault, envelopes e programa não permite testar o PIN offline sem o token, supondo implementação correta e ausência de material capturado em sessão anterior.
- **Riscos/não objetivos:** token roubado com PIN conhecido; malware durante uso; perda do token; recuperação; transporte e memória entre plataformas. Não prometer invulnerabilidade.
- **Teste/oráculo:** copiar estado, retirar token, omitir verificação, restaurar arquivos, substituir destinatário e inspecionar resíduos em fixtures fictícias.
- **Gate:** não alterar o formato de produção antes de validar os caminhos de abertura e recuperação, aparelhos reais e comportamento negativo.

## O que foi realmente testado

Dois programas Go independentes usaram as funções reais de [pkg/crypto](../pkg/crypto/crypto.go) e [DPAPI Windows](../pkg/crypto/device_token_windows.go). Ambos rodaram sob a mesma conta e na mesma máquina.

1. O emissor gerou uma chave aleatória de 32 bytes e cifrou uma frase fictícia com AES-GCM.
2. Protegeu essa chave com Argon2id derivado de `123456` e depois envolveu o resultado com DPAPI.
3. O leitor recebeu somente um JSON com salt, envelope DPAPI e conteúdo cifrado. O processo emissor já havia terminado.
4. O leitor removeu DPAPI, confirmou rejeição de `654321` e abriu o conteúdo fictício com o PIN conhecido `123456`.

Saída observada:

```text
amostra_ficticia_criada=true argon_memoria_kib=65536 argon_passagens=3
outro_executavel_mesmo_usuario_decifrou_dpapi=true
controle_pin_errado_rejeitado=true
pin_conhecido_abriu_conteudo_ficticio_em_outro_executavel=true
```

Os executáveis tinham SHA-256 diferentes. Foram usados DPAPI, Argon2id de 64 MiB/3 passagens e AES-GCM reais; somente o conteúdo e a composição do envelope eram experimentais. Nenhum vault, identidade ou configuração de usuário real foi lido ou alterado.

**Alcance:** demonstra que esse uso de DPAPI não vincula a autorização exclusivamente ao executável emissor. Não é invasão do Kofre atual, não demonstra abertura sem o PIN e não testa outro usuário, outra máquina, extração de memória ou desempenho de força bruta. A documentação descreve a proteção normalmente vinculada ao logon e ao computador, com exceções como perfis móveis. [Microsoft: CryptProtectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata).

## Candidato: token portátil como autorização offline

Fluxo conceitual, ainda não um protocolo de produção:

```text
PIN verificado pelo token + confirmação de presença
                         ↓
resultado secreto de hmac-secret/PRF
                         ↓
derivação com contexto do vault → abre envelope da chave aleatória
                         ↓
                    abre vault.enc
```

No cadastro, o Kofre geraria uma chave aleatória para cifrar o vault e cadastraria uma credencial no token compatível. Os arquivos guardariam metadados públicos, identificadores e a chave do vault cifrada. O segredo interno do token não seria copiado para o PC nem para o S3.

Na abertura, o token verificaria o PIN; o aplicativo usaria a saída secreta para abrir o envelope. Não usar a assinatura pública de autenticação como chave. Não persistir a saída PRF ou a chave aberta para dispensar o token na próxima sessão.

CTAP distingue o segredo usado com verificação do usuário daquele usado sem verificação. Portanto, omitir o PIN no cliente não deve produzir a mesma saída necessária ao envelope criado com verificação. O token também aplica sua política de tentativas; restaurar arquivos do PC não restaura seu contador. É necessário testar modelo, firmware e API escolhidos. [CTAP 2.1, hmac-secret](https://fidoalliance.org/specs/fido-v2.1-ps-20210615/fido-client-to-authenticator-protocol-v2.1-ps-20210615.html#sctn-hmac-secret-extension).

### O que protege e o que continua exposto

| Cenário | Resultado esperado da proposta |
| --- | --- |
| Cópia isolada do vault ou vazamento de armazenamento da nuvem | Falta o segredo necessário do token. |
| Cópia de todos os arquivos do PC, sem resíduos de uma abertura anterior | Falta o token; não deve existir fallback local que o dispense. |
| Token roubado e PIN `123456` conhecido ou adivinhado | Pode abrir na primeira tentativa; limite de erros não impede acerto. |
| Malware dentro do Kofre ou controle suficiente do host durante uso | Pode capturar saída PRF, chave aberta ou conteúdo; token não elimina essa exposição. |
| Cliente alternativo com token e autorização válidos | Pode executar o protocolo. A segurança não depende do nome do executável. |
| Administrador/kernel comprometido | Não há garantia geral para o conteúdo liberado ao host. |

Um arquivo de chave em USB comum também separa o segredo do PC quando está ausente, mas é copiável. Ele não oferece a mesma verificação de PIN nem o contador protegido de um token. É uma alternativa conhecida, não uma inovação nossa.

## Possível contribuição própria: reduzir o impacto de dumps

Hipótese a investigar separadamente: cifrar cada credencial com chave independente e pedir uma operação do token quando ela for usada. Abrir a tela liberaria o catálogo; usar uma senha liberaria apenas o material necessário àquela operação. Não manter na sessão uma chave global capaz de abrir todos os segredos.

Isso pode reduzir a quantidade de conteúdo acessível a um dump pontual, mas ainda não foi demonstrado. Requer mudança substancial de formato e fluxos de importação, exportação, edição e compartilhamento. Catálogo e identificadores também são dados sensíveis. Um atacante que controle a execução pode solicitar outro item ou capturar operações sucessivas. Tokens comuns não mostram qual credencial está sendo autorizada, e hmac-secret admite duas saídas por operação; não prometer “exatamente uma senha por toque”.

Se qualquer cache, chave de identidade ou caminho de recuperação em RAM permitir abrir todos os envelopes, a hipótese perde o benefício. O gate deve inspecionar também chaves intermediárias, não apenas procurar senhas em texto claro.

## Plataformas e integração

Há caminhos documentados, sem validação do Kofre com hardware real:

| Plataforma | Caminho disponível | Pendência relevante |
| --- | --- | --- |
| Windows | libfido2 / proxy Windows WebAuthn para token externo | Operação sem administrador e suporte da extensão. |
| Linux | libfido2 / USB | Permissões, empacotamento e modelos de token. |
| macOS | libfido2 ou SDK nativo | Transporte e interoperabilidade. |
| Android | YubiKit e exemplo oficial de PRF | Aparelhos, USB/NFC, ciclo de sessão. |
| iOS | YubiKit Swift 1.4.0, CTAP2 via NFC | Aplicativo nativo, aparelho real e gestão de memória. |

Fontes: [libfido2](https://developers.yubico.com/libfido2/), [exemplo Android](https://github.com/YubicoLabs/android-prf-sample), [API HmacSecret no Swift 1.4.0](https://github.com/Yubico/yubikit-swift/blob/v1.4.0/YubiKit/YubiKit/FIDO/CTAP/Extensions/HmacSecret.swift), [conexão FIDO no iOS](https://github.com/Yubico/yubikit-swift/blob/v1.4.0/YubiKit/YubiKit/YubiKit.docc/Resources/FIDOConnectionExtension.md), [observações do mantenedor sobre Windows](https://github.com/Yubico/libfido2/discussions/845).

O guia narrativo antigo da Yubico tinha informação de compatibilidade defasada para Swift. A tag 1.4.0 contém a API; isso não prova suporte equivalente no Safari. O [README desse SDK](https://github.com/Yubico/yubikit-swift/blob/v1.4.0/README.md) também declara ausência de zeroização de segredos, relevante para nossos testes de memória. WebAuthn PRF e CTAP puro não devem ser misturados sem normalizar suas transformações de entrada e testar resultados idênticos.

## Recuperação, nuvem e equipes

- Recuperação: segundo token previamente cadastrado ou segredo aleatório independente, guardado separadamente. Nenhum envelope alternativo pode depender somente do PIN curto.
- Sincronização: transportar dados cifrados e envelopes; o servidor não deve adquirir o segredo do token.
- Novo dispositivo ou destinatário: autenticar a chave recebida. Aprovação confiável deve impedir substituição pelo servidor; e-mail confirmado sozinho não autentica uma chave criptográfica.
- Compartilhamento: criar acesso para a chave do destinatário autorizado, sem copiar o segredo interno do token. O desenho completo desse protocolo continua pendente.
- Revogação: protege acesso futuro após rotação apropriada; não apaga cópias já obtidas nem faz uma máquina offline obedecer imediatamente.
- Perda de todas as autorizações e recuperação: perda de acesso. Um reset por e-mail que sozinho devolva os dados eliminaria a fronteira pretendida.

## Gates do próximo protótipo, se a concessão for aceita

1. Abrir fixture offline com token e PIN; rejeitar PIN errado, token diferente e ausência do token.
2. Omitir verificação do usuário: envelope criado com verificação não pode abrir.
3. Reiniciar processo e restaurar todos os arquivos locais: não recuperar autorização sem nova interação autorizada com o token, nem reiniciar seu contador de tentativas.
4. Fazer backup/exportação e testar todos os caminhos de recuperação: nenhum pode criar alternativa de PIN curto apenas.
5. Executar sem privilégio administrativo e validar interoperabilidade nos sistemas anunciados.
6. Inspecionar logs, temporários, cache e dumps em dados fictícios durante e após operações. Ausência de uma string num dump isolado não prova ausência de segredos.
7. Substituir destinatário, contexto do vault e envelope: adulterações precisam ser rejeitadas.
8. Para a hipótese por credencial, usar lote fictício e verificar o que o material extraído realmente permite decifrar; rejeitar a arquitetura se uma chave da sessão liberar todos os itens.
9. Adulterar metadados de credencial/verificação ou usar token sem a extensão exigida: recusar a operação, sem fallback para PIN apenas, armazenamento local ou autorização sem verificação do usuário.

## Recomendação

Apresentar o token portátil como opção de proteção adicional, sujeito à aceitação da exigência física. Se a exigência continuar sendo somente software, offline, PIN previsível e nenhum segredo externo, informar que a proteção desejada contra cópia completa não foi alcançada. Não reduzir a promessa a uma combinação de nomes técnicos.

Este estudo não altera o Kofre, seus cofres existentes, regras de senha ou serviço Cloud.
