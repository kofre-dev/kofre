# Instalação e desinstalação no Windows

Versão 1.0.8: instalação por usuário em `%LOCALAPPDATA%\Programs\Kofre`,
registro em Aplicativos instalados e comando `kofre uninstall`/`--uninstall`.
O script público chama `kofre install`, mostra como abrir novamente e inicia
o aplicativo em uma nova janela. O comando CMD abreviado continua usando
PowerShell internamente, sem exigir que o usuário abra esse terminal.

## Decisão de engenharia

Problema: instalação sem entrada de desinstalação; evidência: o script apenas
baixava o executável e alterava PATH; invariante: nunca apagar vault, configuração,
backup ou diretórios recursivamente; solução: entrada de desinstalação HKCU e
helper PowerShell oculto que espera o processo sair e remove somente `Kofre.exe`,
a entrada exata do PATH e a chave de registro do aplicativo; previsão: o aplicativo
desaparece da lista sem afetar dados; riscos: outras instâncias abertas podem
impedir a exclusão; oráculo: teste real em diretório e chaves HKCU isolados;
gate: executável e registro removidos, PATH vizinho e dados fictícios intactos.

O helper tenta remover o executável por até 15 segundos após esperar a saída do
processo que iniciou a desinstalação. O resultado fica em
`%TEMP%\Kofre-desinstalacao.log`. Ele não encerra à força outras instâncias.
O diretório e arquivos adicionais são preservados, inclusive backups de binários.
Não existe opção implícita para apagar credenciais nem exclusão de dados na nuvem.
Os comandos install/uninstall são tratados antes da limpeza de chaves legadas
e da atualização automática, para não tocar dados durante essas operações.

Instalações existentes ganham o registro ao iniciar o executável atualizado no
caminho padrão. Executáveis portáteis não se registram automaticamente.

Validação: `go test ./pkg/installer` executa o helper real com nomes contendo
espaços, apóstrofos e colchetes. Usa registro próprio temporário, sem alterar
a instalação pessoal nem seu PATH. A sintaxe do script público foi validada
pelo parser PowerShell, sem executá-lo. A tentativa de executar um harness com
chamadas simuladas foi bloqueada pela política automática da ferramenta; não foi
contabilizada como teste concluído. A abertura visual de um terminal novo após
instalação continua sendo uma verificação manual.

Referência: [propriedades de desinstalação do Windows](https://learn.microsoft.com/en-us/windows/win32/msi/uninstall-registry-key).

## Publicação verificada em 2026-10-02

Versão Windows x64 1.0.8 publicada pelo publisher. SHA-256 do download público:
`cd62ed689b2170881137878edf26125acc317e34dd2d55b91b08a6afaed0e80f`.
Os testes com detector de concorrência e `go vet` passaram no cliente; os testes
e `go vet` do servidor também passaram. O download público foi comparado com
o hash do artefato local e com os metadados de versão.

Landing e instalador implantados no Railway, serviço `kofre-cloud`, produção,
deployment `ec2b044b-2a51-4b5e-9785-e63d2ce1035f`, estado SUCCESS. A página pública
contém o comando CMD abreviado e a quebra de linha; o script público contém o
registro via `install`, a dica de uso e a chamada para abrir uma nova janela.
