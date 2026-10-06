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
Riscos/não objetivos: não há recuperação por e-mail nem alteração de API,
cobrança ou criptografia do servidor. Exportações antigas mantêm a senha usada
na data da exportação. A chave da conta existe em memória enquanto uma ação
corporativa a utiliza, como no fluxo anterior; não se promete inacessibilidade
em máquina comprometida.
Oráculo: round-trip cifrado, troca de senha na TUI, exportação/abertura com a
senha mestra, rejeição de senha errada, migração e preservação de backup,
ausência de identidade em busca/exportação de item/variáveis de ambiente.
Gate: testes e vet do cliente aprovados; nenhum cofre real aberto para testes.

## Funcionamento

- Primeiro crie/abra seu cofre local. Em `Criar conta gratuita`, confirme a
  senha mestra e informe seu nome. A sincronização continua sendo uma escolha.
- O cadastro não solicita nem cria uma segunda senha. Token e chaves são
  gerados no dispositivo; a senha mestra não é enviada ao Cloud.
- O menu de conta roda em processo separado: por isso confirma a mesma senha,
  sem transportar segredos por argumentos ou variáveis de ambiente.
- A primeira gravação com conta preserva um backup cifrado do cofre anterior.
- Contas antigas são importadas uma vez usando a senha anterior. O arquivo
  antigo permanece intacto como backup; os acessos seguintes usam o cofre.
- `empresa exportar` exporta só a identidade com a senha mestra atual. A
  restauração pede a senha usada ao exportar e passa a proteger a identidade
  com a senha do cofre de destino. Arquivos existentes não são sobrescritos.
- Restaurar histórico conserva a identidade atual, sem reverter tokens/chaves.

## Compatibilidade

O cliente 1.0.18 lê KOFRE001, MYCOFRE1 e KOFRE002. Cofres sem identidade
continuam sendo gravados como KOFRE001. Cofres com identidade usam KOFRE002:
clientes anteriores recusam o cabeçalho, em vez de ignorar e apagar o campo
desconhecido ao salvar. Atualize todos os PCs antes de usar o cofre com conta.
O Cloud conserva o conteúdo cifrado como opaco; não requer mudança de API.
Esta entrega gera build local; publicação de release é uma etapa separada.

A gravação verifica se o arquivo mudou desde a abertura e recusa uma base
desatualizada. Isso não substitui um bloqueio global entre processos; continua
existindo a limitação anterior de abrir o mesmo cofre simultaneamente em
processos que não coordenam a gravação. Conflitos remotos seguem o controle
de revisão da sincronização existente.
