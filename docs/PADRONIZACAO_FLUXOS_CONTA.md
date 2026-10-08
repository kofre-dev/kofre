# Padronização dos fluxos de conta e empresas

## Decisão — 08/10/2026

Problema: conta, organização, convite e assinatura alternavam menus visuais
com perguntas e resultados no console. Evidência: `painelConta` e
`painelEmpresa` chamavam `fmt.Println`, `ReadPassword` e `ReadString` entre
seletores independentes. Causa: o menu era TUI, mas sua camada de interação
continuava textual.

Invariantes: senha em buffers apagáveis; autenticação, limitador de tentativas,
cifra e confirmação de fingerprints preservados; preços e autorização no
Cloud; cancelamento não inicia a próxima alteração.

Solução: uma instância Bubble Tea mantém a tela alternativa durante todo o
painel. A lógica sequencial pede seleção, campo, confirmação, resultado ou
espera pela camada de interação. O runner aceita um leitor seguro fornecido
pelo painel, utilizando o mesmo KDF e limitador. Não há captura global de
stdout nem conversão da senha para strings de exibição.

Previsão: fluxos permanecem no padrão de marca, paleta, caixa, rodapé e
atalhos; resultados longos podem ser rolados; Esc cancela o campo ou a espera
do pagamento. Comandos explícitos da CLI conservam sua apresentação textual.

Riscos e limites: Esc não desfaz uma operação já confirmada no servidor. Nas
operações síncronas de rede, a tela indica processamento e aguarda os limites
de timeout já existentes. O painel fecha o cofre principal antes de abrir o
processo de conta, como antes. Revelação explícita continua temporária, com
limpeza do terminal; capturas externas não são controladas pelo aplicativo.

Oráculos: testes de fluxo com um único par de entrada/saída da tela alternativa;
senha ausente da renderização; limpeza na confirmação, Esc e inatividade;
espera encerra seu contexto antes de retornar; cancelamento de workspace,
convite, assentos e renovação não envia alteração HTTP; valores da cotação
vêm da resposta simulada do Cloud; layout cabe em 40/80/120 colunas.

Gate: testes e detector de corridas aprovados nos pacotes alterados, suite do
cliente e `go vet` aprovados, binário Windows compilado. Os testes de cobrança
usam servidor fictício, sem pagamento real ou criação de contas em produção.

## Verificação realizada

Suite do cliente, `go vet ./...` e `go test -race` de `cmd/kofre`, `pkg/tui`
e `pkg/runner` aprovados. Smoke do executável Windows no terminal: abriu
Conta e empresas, entrou no campo de e-mail, voltou com Esc ao menu e saiu
com Esc, sem prompt textual intermediário e sem enviar e-mail. A autenticação
e a compra reais não foram repetidas nesta revisão.

Preview: `dist/preview-fluxos/kofre.exe`. As correções anteriores foram
instaladas localmente; publicação remota e commit ainda não foram realizados.

## Menu resumido e explicações

O início apresenta cinco opções: ativar nuvem gratuita, entrar na conta,
empresas e equipes, conhecer planos e voltar ao cofre. Com sincronização
configurada, as duas primeiras aparecem como Sincronização e Minha conta.
Esses nomes são apresentação; a autorização continua no Cloud.

Cada opção tem uma caixa de explicação que acompanha as setas. Navegar não
executa a ação: Enter confirma e Esc volta. Sessões, backups e recuperação
ficam em Minha conta; Pro e contratação corporativa ficam em Conhecer planos.
Uma conta existente pode ativar a nuvem sem repetir cadastro. Sem identidade,
empresas e contratação oferecem cadastro ou entrada antes de continuar.

Conta criada e sincronização habilitada são estados distintos. O indicador
local `conta_configurada` apenas ajusta nomes do menu; não guarda chave,
token ou senha e não autoriza acesso. Recusar a sincronização conserva o
cofre offline. A home exibe ausência de sincronização, envio pendente ou
nuvem conectada sem envios pendentes, consultando o worker sob mutex.
Esse indicador não garante conectividade em tempo real.

O formulário começa pela categoria e mantém a senha vazia até digitação ou
geração. Notas ficam legíveis durante criação/edição por escolha de UX;
continuam cifradas no arquivo e em buffers apagáveis no modelo. Detalhes e
listagem mantêm as notas protegidas até revelação explícita. Renderizar notas
para edição permite que o terminal retenha o texto exibido, como nos demais
campos visíveis. Senhas continuam mascaradas em todas essas telas.

Empresas e equipes apresenta Minhas empresas, Criar minha empresa e
Identificação e backup, além de Voltar. Convites recebidos fica em Minha
conta: a empresa convida pelo e-mail; a pessoa autenticada aceita ou recusa
no aplicativo sem copiar código. As ações de cada empresa
ficam em Credenciais, Equipe e workspaces e Assinatura. Navegar ou voltar
não executa comandos; as permissões continuam sendo verificadas no servidor.

Telegram: o Free conectado tentava registrar um envelope e recebia HTTP 403.
Invariante: sincronização Free não ativa desbloqueio remoto nem concede Pro.
A tentativa automática exige preferência Telegram, nuvem e plano Pro; a API
continua autorizando o registro. Falhas aplicáveis ficam visíveis sem fechar
o cofre. Testes cobrem Free, preferência desativada, Pro e erro persistente.
