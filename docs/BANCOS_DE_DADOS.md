# Credenciais de bancos de dados

No cofre, pressione `n`, escolha **Banco de dados** na categoria com `←/→`
e percorra os campos com `Tab` ou `Enter`. Use `Ctrl+S` para salvar.
Em terminais menores, o formulário acompanha o campo selecionado.

Tipos disponíveis: PostgreSQL, MySQL, MariaDB, SQL Server, Firebird, Oracle,
MongoDB, Redis, SQLite e Outro. O tipo e a configuração TLS também podem ser
selecionados com `←/→`. A porta sugerida acompanha a troca de tipo, preservando
uma porta personalizada. O host é obrigatório; a porta é opcional e, quando
informada, deve estar entre 1 e 65535. Nome do banco/serviço, usuário, senha
e observações são opcionais. SQLite exige o caminho do arquivo e apresenta
somente os campos pertinentes ao arquivo.

`Ctrl+G` no campo de senha gera uma senha forte. A senha permanece oculta;
na edição, deixar esse campo vazio preserva o segredo existente. Nos detalhes,
é possível selecionar os campos para copiar ou revelar temporariamente.
A categoria **Bancos de dados** filtra a lista, e a busca encontra host,
nome do banco e usuário sem pesquisar o valor da senha.

TLS é uma configuração registrada para uso ao conectar: o Kofre não abre
conexões de banco, testa servidores nem valida certificados. A opção inicial
é **Validar certificado**. Não salve senhas dentro do host, do caminho ou de
outros campos visíveis; use o campo de senha dedicado.

Os campos integram o formato existente de credenciais, sem um arquivo de
conexão separado. O cofre completo continua cifrado; senhas usam o buffer
protegido existente em memória. Metadados como host e usuário são visíveis
com o cofre aberto, seguindo o tratamento das credenciais atuais.
A sincronização envia o cofre cifrado. Para compartilhar com uma empresa,
envie explicitamente essa entrada pelo fluxo de credenciais da organização;
o compartilhamento mantém as permissões e a cifra por destinatário existentes.

## Registro de engenharia

Problema: conexões sem campos próprios; evidência: formulário genérico de
usuário, segredo e notas; invariante: nenhuma senha entra na renderização,
nenhuma outra entrada pessoal é compartilhada; causa: ausência da categoria
e dos campos de conexão; referência: `SecretEntry.Fields`, `NewProtectedField`
e `ExportarEntrada`; solução: categoria `database` e metadados no contrato
existente, com senha no caminho protegido; previsão: salvar, editar, reabrir,
buscar e compartilhar conexões sem alterar o protocolo criptográfico;
riscos/não objetivos: dados visíveis após desbloqueio, sem conexão ativa ou
execução de comandos de banco; oráculo: reabertura da cifra, ausência de texto
nos bytes persistidos, senha preservada na edição, validação antes da gravação
e integração corporativa com tentativa de leitura por terceiro; gate:
homologação do cliente e Cloud, com sentinelas de formulário obrigatórios.
