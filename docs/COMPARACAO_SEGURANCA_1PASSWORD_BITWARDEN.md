# Comparação de desbloqueio: 1Password, Bitwarden e Kofre

Data: 07/10/2026. Escopo: leitura de fontes públicos, documentação oficial e obtenção dos pacotes oficiais para estudo. Testes interativos e ataques contra cofres fictícios ainda não executados.

Pacotes baixados pelos links dos sites oficiais, sem instalação ou execução:

| Pacote | Versão identificada | Assinatura Authenticode |
| --- | --- | --- |
| 1Password MSIX bundle | 8.12.40.31 | Válida; Agilebits |
| Instalador Bitwarden | 2026.9.1 | Válida; Bitwarden Inc. |

SHA-256 do bundle 1Password: `3455AE98708F4BD2FEF91E96FC00A5855CFF59A11E20CE2B4498E1BD298F3DA6`.

SHA-256 do instalador Bitwarden: `49B10F0840E70BDD1701CC9A0B729813015A70D3D3B487682A4E4A93819A3263`.

O instalador Bitwarden é pequeno e pode obter componentes na instalação. A verificação desses arquivos não equivale à inspeção de um aplicativo instalado. Fontes de download: [1Password](https://1password.com/downloads/windows) e [Bitwarden](https://bitwarden.com/download/).

## Evidência e limites

O Bitwarden foi examinado na tag `desktop-v2026.9.1`, commit `8246ae9c9a484a0a69f8b27203034555fb872523`. O `package-lock.json` fixa o SDK JavaScript em `0.2.0-main.1034`; os metadados desse pacote apontam para o commit `7de8f13a14b56068167160f88d55231f916cf16a` do SDK. O componente nativo tem sua própria dependência Rust, fixada no Cargo.lock; não confundir as duas versões.

Fontes: [package-lock da versão](https://github.com/bitwarden/clients/blob/8246ae9c9a484a0a69f8b27203034555fb872523/package-lock.json), [metadados do SDK publicado](https://registry.npmjs.org/@bitwarden%2Fsdk-internal/0.2.0-main.1034).

O 1Password principal não é open source, embora publique componentes e documentação de segurança. O Go SDK público encaminha chamadas a um núcleo WASM distribuído como binário; sua disponibilidade não revela a implementação do desbloqueio desktop. O Electron hardener público protege configurações do aplicativo, sem constituir o núcleo do cofre. [Declaração oficial](https://www.1password.community/cybersecurity-glossary-67/open-source-security-24725), [SDK: núcleo WASM](https://github.com/1Password/onepassword-sdk-go/blob/5866f43111ffeee5952e43a13da1aafef98200c8/internal/extism_core.go#L12-L23), [hardener](https://github.com/1Password/electron-hardener/blob/52deabd307186cbae10f04c8bd4467a9543b4787/src/lib.rs#L1-L19).

## 1. PIN do Bitwarden: caminho confirmado no código

A tela de configuração escolhe `AfterFirstUnlock` quando exige senha mestra ao reiniciar; caso contrário, escolhe `BeforeFirstUnlock`. O serviço TypeScript repassa a configuração ao SDK. [Tela](https://github.com/bitwarden/clients/blob/8246ae9c9a484a0a69f8b27203034555fb872523/libs/angular/src/auth/components/set-pin.component.ts#L48-L51), [serviço](https://github.com/bitwarden/clients/blob/8246ae9c9a484a0a69f8b27203034555fb872523/libs/common/src/key-management/pin/pin.service.implementation.ts).

O SDK protege a chave do usuário com um envelope derivado do PIN. Sempre prepara o envelope efêmero; só persiste o envelope quando o modo é `BeforeFirstUnlock`. Para abrir, deriva a chave do PIN e autentica/decifra o envelope. [Implementação de PIN](https://github.com/bitwarden/sdk-internal/blob/7de8f13a14b56068167160f88d55231f916cf16a/crates/bitwarden-core/src/key_management/pin_lock_system.rs#L345-L385).

Na suíte padrão desktop, o envelope usa Argon2id com **64 MiB, 3 passagens e paralelismo 4**, salt aleatório e chave de 32 bytes. São os mesmos custos Argon2 configurados atualmente no Kofre. O envelope novo usa AES-256-GCM; há caminho FIPS com PBKDF2 e tratamento diferente para iOS. Essa comparação é do envelope de PIN, não uma afirmação de que todo KDF do Bitwarden usa os mesmos valores. [Envelope e parâmetros](https://github.com/bitwarden/sdk-internal/blob/7de8f13a14b56068167160f88d55231f916cf16a/crates/bitwarden-crypto/src/safe/password_protected_key_envelope.rs#L420-L465), [Kofre](../pkg/crypto/crypto.go).

**Inferência:** obter o envelope persistente fornece material para testar candidatos ao PIN fora da interface. O logout após cinco erros é uma decisão do cliente, não uma propriedade que impeça cálculos sobre uma cópia dos dados. O próprio fabricante alerta para o enfraquecimento da proteção local ao usar PIN. [Contador no cliente](https://github.com/bitwarden/clients/blob/8246ae9c9a484a0a69f8b27203034555fb872523/libs/key-management-ui/src/lock/components/lock.component.ts#L494-L508), [documentação do PIN](https://bitwarden.com/help/unlock-with-pin/).

## 2. Windows Hello é um caminho diferente

O Bitwarden possui uma rota com chave efêmera em memória protegida e outra persistente que usa uma operação de assinatura do Windows Hello para derivar o segredo que protege a chave do usuário. A segurança passa a depender dessa autorização do sistema e do armazenamento da chave correspondente. Não equivale a simplesmente cifrar um arquivo com o PIN do aplicativo. O código reconhece a necessidade de isolamento contra injeção. [Implementação Windows da versão](https://github.com/bitwarden/clients/blob/8246ae9c9a484a0a69f8b27203034555fb872523/apps/desktop/desktop_native/biometric/src/windows.rs).

Nessa mesma versão, a implementação Linux usa autorização polkit e memória; `has_persistent` retorna falso. Não existe garantia homogênea de persistência biométrica nas plataformas. [Implementação Linux](https://github.com/bitwarden/clients/blob/8246ae9c9a484a0a69f8b27203034555fb872523/apps/desktop/desktop_native/biometric/src/linux.rs#L55-L85).

O 1Password documenta que, sem TPM, mantém o segredo de desbloqueio Hello na memória e exige a senha principal após encerrar o aplicativo ou reiniciar o PC. Com TPM, pode manter o desbloqueio após reinício. [Documentação de segurança do Windows Hello](https://support.1password.com/windows-hello-security/).

## 3. Secret Key do 1Password

A senha principal é combinada com uma Secret Key aleatória que o usuário não precisa memorizar. A Secret Key fica nos dispositivos cadastrados e no kit de emergência; protege especialmente cópias obtidas do servidor. Quando o atacante obtém o conjunto de dados locais necessário, a senha principal continua relevante. Portanto, separar uma chave do servidor melhora a proteção da nuvem, mas não elimina automaticamente o problema da cópia completa do dispositivo. [Secret Key](https://support.1password.com/secret-key-security/).

## 4. Decifrar somente quando necessário

A arquitetura documentada das contas 1Password cifra separadamente o resumo e os detalhes dos itens usando a chave do cofre. Isso permite listar títulos e URLs sem decifrar senhas e notas. É uma prática estabelecida de redução da exposição do conteúdo em memória. Quem obtém a chave do cofre ainda pode decifrar os itens disponíveis. Não confundir isso com autorização independente por credencial ou proteção contra um processo inteiramente comprometido. [Whitepaper: proteção de itens](https://agilebits.github.io/security-design/secureItems.html).

A especificação OPVault anterior tinha organização de chaves diferente. Ela não deve ser apresentada como prova da implementação das contas atuais.

## O que isso muda para o Kofre

1. Os custos Argon2 atuais do Kofre não estão isoladamente fora do padrão observado. Isso não comprova a segurança global do aplicativo nem fortalece um PIN previsível.
2. Senha principal e desbloqueio rápido têm papéis distintos nos produtos analisados. A conveniência envolve uma chave já autorizada, uma proteção do sistema ou uma concessão explícita na proteção local.
3. Vale investigar separar catálogo e detalhes no formato do Kofre para evitar decifrar todas as credenciais durante listagem. Antes, mapear efeitos em edição, salvamento, importação, compartilhamento e memória.
4. Nenhuma evidência encontrada justifica prometer `123456` como único segredo seguro, offline, depois de reiniciar e contra cópia completa do estado local.

## Próxima validação prática

Usar instalação isolada e dados exclusivamente fictícios. Registrar versão e configuração. Comparar PIN de sessão, PIN persistente e autenticação do sistema: bloqueio, encerramento completo, abertura offline, reinício e cópia dos arquivos de teste. Inspeção de memória deve procurar tanto conteúdo quanto chaves capazes de abri-lo. Ausência de strings em um dump isolado não comprova proteção.

O Bitwarden documenta uso offline em modo somente leitura; não equiparar esse comportamento ao uso local completo do Kofre. [Uso offline](https://bitwarden.com/help/using-bitwarden-offline/).

Nenhuma conta real, instalação existente ou cofre do usuário foi usado no estudo. Nenhuma mudança de segurança foi aplicada ao Kofre como resultado desta comparação.
