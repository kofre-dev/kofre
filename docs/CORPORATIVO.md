# Kofre — conta e empresas

O cofre pessoal permanece separado. Uma participação na empresa não exige
Pro pessoal: a autorização corporativa é consultada no Cloud. Regras comerciais
e assentos são controlados pelo backend privado.

## Preparar uma pessoa

Abra `kofre conta` ou pressione **Ctrl+E após desbloquear o cofre**. O menu permite
criar uma conta gratuita, conectar a nuvem pessoal, administrar empresas,
restaurar a identidade e consultar o histórico pessoal. Pelo terminal:

```
kofre empresa criar-conta
kofre empresa conectar
kofre empresa fingerprint
kofre empresa exportar D:\Backup\identidade-empresa.enc
```

A conta usa a mesma senha mestra do cofre. Nome, token, recuperação e chaves
ficam dentro do cofre cifrado; não há uma segunda senha para criar ou lembrar.
A identidade antiga pode ser migrada uma vez com a senha anterior, preservando
o arquivo original. Veja [senha única](CONTA_SENHA_UNICA.md) para backup,
restauração e compatibilidade entre versões.

Se o registro da chave falhar após salvar o arquivo, execute
`kofre empresa publicar-chave`. Não gere outra chave para a mesma identidade.
No novo computador, use `kofre empresa restaurar <backup.enc>` antes de
`kofre empresa recuperar`. A recuperação troca o token, mantendo ID e chaves.
Se falhar, repita: a solicitação pendente está no arquivo cifrado. Exporte um
backup atualizado após configurar, recuperar ou confirmar novas identidades.

## Entregar uma credencial

No menu, escolha **Contratar para uma nova empresa** ou execute `kofre empresa
comprar`. O navegador recebe uma compra vinculada à sua identidade. Mantenha o
aplicativo aberto. A organização é liberada após confirmação financeira.

O proprietário convida pessoas com conta gratuita; o destinatário aceita usando
o código do convite. Participar da empresa não altera o plano pessoal:

```
kofre empresa listar
kofre empresa convidar <org> <pessoa>
kofre empresa aceitar <org>
kofre empresa workspace <org> "Projeto A"
kofre empresa enviar <org> <workspace> <id-da-entrada-pessoal>
kofre empresa compartilhar <org> <item> <pessoa>
kofre empresa mostrar <org> <item>
```

A entrada enviada é uma cópia explícita. Ela permanece no cofre pessoal e
alterações posteriores não se propagam automaticamente. O conteúdo fica cifrado
por destinatário com HPKE (X25519, HKDF-SHA256, AES-256-GCM) e assinado com
Ed25519. Compare o fingerprint com o resultado de `empresa fingerprint` da outra
pessoa por um canal independente antes de confirmar envio ou primeira leitura.
O fingerprint confirmado fica no arquivo cifrado; troca inesperada é recusada.

## Equipes e permissões

```
kofre empresa equipe <org> "Desenvolvimento" <pessoa1,pessoa2>
kofre empresa compartilhar-equipe <org> <item> <equipe>
kofre empresa atualizar-equipe <org> <equipe> "Desenvolvimento" <ids>
kofre empresa recompartilhar <org> <item>
kofre empresa permissao <org> <item> <pessoa> editor
kofre empresa editar <org> <item> <id-da-entrada-pessoal>
```

Leitor abre; editor atualiza conteúdo; gestor altera compartilhamento. Novos
integrantes da equipe só recebem o item após recompartilhamento. Remoção bloqueia
novas leituras e retira a cópia armazenada no Cloud. Não apaga senhas ou arquivos
que a pessoa já copiou. Credenciais sensíveis devem ser trocadas na origem.
Transferir propriedade da organização não concede leitura de itens.
O proprietário pode delegar administração com
`kofre empresa papel <org> <pessoa> administrador`. O administrador gere membros,
equipes e workspaces, sem alterar assentos contratados, promover administradores,
remover o proprietário ou ganhar acesso ao conteúdo cifrado.

## Limites da primeira entrega

Os recursos corporativos estão disponíveis no menu e nos comandos `empresa`.
Não há sincronização corporativa offline automática. API e cliente exigem conexão
para operar. Organização, equipes, membros, ACLs e disponibilidade dependem do Cloud;
o servidor não recebe chave privada nem conteúdo aberto. Assinaturas verificam
autoria e integridade das cópias, mas não constituem log transparente de versões
nem proteção contra um servidor que reapresente uma versão antiga inteira.

Guarde o arquivo cifrado e sua senha. Recuperar autenticação não reconstrói chave
privada perdida. Sem backup ou outra pessoa autorizada capaz de recompartilhar,
o suporte não consegue recuperar o conteúdo. Revogação administrativa não pode
ser desfeita pelo segredo de recuperação.

## Assinatura e assentos

O proprietário consulta o contrato no menu ou com:

```
kofre empresa assinatura <organização>
kofre empresa assentos <organização> <quantidade-total>
kofre empresa renovar <organização>
kofre empresa cancelar-assinatura <organização>
```

O Cloud informa o valor antes da confirmação. Assentos adicionais são liberados
após pagamento proporcional. Reduções entram na renovação e precisam comportar
os membros ativos. A recorrência pode ser cancelada mantendo o período pago;
contratos anuais têm renovação manual. Valores e regras pertencem ao Cloud.
Uma fatura futura já paga exige aguardar sua vigência antes de mudar assentos.

## Nuvem pessoal e histórico

Free sincroniza o arquivo pessoal após conectar a conta. Pro acrescenta histórico
e Telegram; o menu mostra o plano confirmado pelo Cloud. `kofre historico` lista
versões e `kofre historico restaurar <versão>` solicita a senha daquela versão,
confirma a restauração e preserva backup cifrado local. Alterações concorrentes
em computadores diferentes geram conflito em vez de substituir silenciosamente
a cópia remota. Confira e preserve ambas antes de escolher uma versão.

## Disponibilidade desta prévia

Implementação local em validação para lançamento. A oferta depende da habilitação
no servidor; o build local não muda o serviço de produção. Para homologação, use
`KOFRE_CLOUD_ENDPOINT` com a URL HTTPS do ambiente preparado. Um pagamento Sandbox
confirma o teste e não concede contrato de produção. Guarde backups cifrados e
use dados fictícios na homologação.
