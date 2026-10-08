# Homologação do Kofre

## Rotina

No checkout público, execute:

```powershell
python scripts/homologar.py
```

Para a homologação completa, mantenha os checkouts `kofre` e `kofre-cloud`
lado a lado:

```powershell
python scripts/homologar.py --cloud ../kofre-cloud --repeticoes 5
```

O modo somente cliente não certifica as regras do servidor. A homologação
completa executa `go vet`, a suite inteira com detector de corridas e cinco
repetições dos testes direcionados de ambos os módulos. Um teste obrigatório
ausente, pulado ou incompleto reprova a rotina, mesmo que `go test` retorne zero.

Relatório e logs ficam em `build/homologacao`, ignorados pelo Git. O relatório
registra commits quando disponíveis, hash das fontes, alterações locais, escopo, comandos, repetições, contagem de
aprovações, tempos e resultado. Todos os usuários e segredos são fictícios.
No container, o Cloud não contém `.git`: seu commit fica nulo no relatório,
e o hash das fontes identifica o conteúdo efetivamente testado.

## Regras protegidas

- Isolamento entre usuários e organizações, inclusive IDs de itens iguais.
- A mesma pessoa participa de duas organizações, mantendo acessos separados.
- Leitor não edita/exclui; editor não concede permissões; administrador
  delegado não altera contrato nem promove outros administradores.
- Convite não pode ser consumido por outra pessoa; cancelamento e revogação
  fecham acesso; remoção de uma empresa não remove a identidade pessoal.
- Entrada em equipe não cria uma cópia criptográfica inexistente.
- Assinaturas criptográficas vinculam organização, item, destinatário e versão.
- Assentos concorrentes respeitam a capacidade; pagamentos e estornos não
  concedem Pro pessoal por acidente; Sandbox não concede licença real.
- Free sincroniza sem ganhar Pro; sessões e backups respeitam identidade.
- Senhas não entram na renderização; cancelamento não envia alterações.

## Automação e publicação

O workflow do cliente roda nos eventos configurados de push, pull request e
execução manual. A construção dos artifacts depende da homologação aprovada.
O workflow do Cloud roda em todos os pushes e pull requests, em Windows e
Linux, e faz checkout do cliente público para a integração real.

O build Docker do Cloud também executa a homologação completa antes de gerar
o servidor. Isso protege o deploy pelo Railway CLI, que poderia acontecer
antes de um workflow do GitHub terminar. O argumento `KOFRE_CLIENT_REF` pode
fixar o SHA do cliente; o padrão é `main`, com SHA efetivo no relatório.

Ao entregar mudança nos dois repositórios, publique primeiro o cliente:
o Cloud depende da rotina e dos testes existentes nesse checkout público.
Sem essa dependência disponível, o gate falha e não gera o servidor.

A proteção de merge na branch `main` deve exigir os checks de homologação.
Essa configuração é administrativa no GitHub, separada dos arquivos de CI.
Não confunda "workflow configurado" com "merge obrigatório configurado".

## Manutenção

Uma regra nova exige um cenário positivo e a tentativa proibida correspondente.
Adicione testes sentinela à lista de obrigatórios do script quando representarem
uma fronteira de autorização. Renomear um sentinela exige atualizar essa lista;
nunca remova a obrigatoriedade apenas para contornar uma falha.

Teste aprovado é evidência dos cenários exercitados, não prova de ausência
de todas as vulnerabilidades. A rotina não usa produção nem substitui testes
reais de infraestrutura em ambiente separado.

Verificação inicial: homologação completa aprovada em Windows (cinco
repetições) e no Docker Linux pelo WSL (três repetições), usando fontes locais.
Os cinco testes do próprio gate também passaram. O CI remoto ainda precisa
receber os arquivos via commit/push para ativar esta rotina.
