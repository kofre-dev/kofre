# Conta protegida pela senha mestra

## Decisão

Problema: cadastro exigia uma senha para identidade além da senha do cofre;
trocar apenas o texto manteria duas proteções independentes.
Evidência: `executarEmpresa` abria `identidade-empresa.enc` e `changePassword`
recriptografava somente o cofre.
Invariante: conta e credenciais passam pela mesma troca atômica do arquivo
cifrado; conta não é uma credencial listável/compartilhável.
Solução: identidade em campo protegido da coleção, selado em memória;
operações de conta autenticam com a senha mestra e persistem no cofre ativo.
Previsão: trocar a senha protege ambos; exportar identidade usa chave/salt do
cofre com nonce aleatório próprio, sem armazenar a senha em texto.
Riscos/não objetivos: não há redefinição da senha mestra por e-mail. A versão
1.0.19 integra as rotas de confirmação e sessões. Exportações antigas mantêm a senha usada
na data da exportação. A chave da conta existe em memória enquanto uma ação
corporativa a utiliza, como no fluxo anterior; não se promete inacessibilidade
em máquina comprometida.
Oráculo: round-trip cifrado, troca de senha na TUI, exportação/abertura com a
senha mestra, rejeição de senha errada, migração e preservação de backup,
ausência de identidade em busca/exportação de item/variáveis de ambiente.
Gate: testes e vet do cliente aprovados; nenhum cofre real aberto para testes.

## Funcionamento

- Primeiro crie/abra seu cofre local. Em `Criar conta gratuita por e-mail`, confirme
  a senha mestra, informe nome/e-mail e digite o código recebido. A nuvem é opcional.
- O cadastro não solicita nem cria uma segunda senha. Token e chaves são
  gerados no dispositivo; a senha mestra não é enviada ao Cloud.
- O menu de conta roda em processo separado: por isso confirma a mesma senha,
  sem transportar segredos por argumentos ou variáveis de ambiente.
- A primeira gravação com conta preserva um backup cifrado do cofre anterior.
- Contas antigas são importadas uma vez usando a senha anterior. O arquivo
  antigo permanece intacto como backup; os acessos seguintes usam o cofre.
- `kofre conta entrar` recebe código por e-mail e pede a mesma senha mestra,
  prova a chave privada e obtém uma sessão de 30 dias. Destino com identidade
  diferente ou senha incompatível é recusado; destino vazio pode baixar o cofre.
- `kofre conta recuperar` exige e-mail e identidade protegida com o segredo
  independente de recuperação. Não redefine a senha nem recupera chave perdida.
- `kofre conta sessoes` permite revogar dispositivos; `kofre conta sair` encerra
  a sessão deste PC. Cadastro confirmado exige assinatura e emite a primeira
  sessão de 30 dias; o token usado apenas para solicitar cadastro é aposentado.
- `kofre conta backup` publica a identidade cifrada. Trocar a senha de uma conta
  por e-mail exige conexão; falha após gravar a nova senha mantém a atualização
  pendente dentro do cofre e tenta novamente na próxima abertura. Conflito remoto
  não é sobrescrito silenciosamente. Uma conta com pendência não troca a senha de novo.
- `empresa exportar` exporta só a identidade com a senha mestra atual. A
  restauração pede a senha usada ao exportar e passa a proteger a identidade
  com a senha do cofre de destino. Arquivos existentes não são sobrescritos.
- Restaurar histórico conserva a identidade atual, sem reverter tokens/chaves.

## Compatibilidade

O cliente 1.0.19 lê KOFRE001, MYCOFRE1 e KOFRE002. Cofres sem identidade
continuam sendo gravados como KOFRE001. Cofres com identidade usam KOFRE002:
clientes anteriores recusam o cabeçalho, em vez de ignorar e apagar o campo
desconhecido ao salvar. Atualize todos os PCs antes de usar o cofre com conta.
O Cloud conserva o conteúdo do cofre cifrado como opaco. As rotas de conta
por e-mail exigem a confirmação assinada e fornecem sessões por dispositivo.
Com e-mail habilitado, cadastro e recuperação públicos anteriores respondem 410.

A gravação verifica se o arquivo mudou desde a abertura e recusa uma base
desatualizada. Isso não substitui um bloqueio global entre processos; continua
existindo a limitação anterior de abrir o mesmo cofre simultaneamente em
processos que não coordenam a gravação. Conflitos remotos seguem o controle
de revisão da sincronização existente.
