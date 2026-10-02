# Monetização Pro e Enterprise — proposta de lançamento

Atualizado em 02/10/2026. Pesquisa de fontes primárias; preços sujeitos a mudanças. Este documento substitui as recomendações comerciais do antigo `PLANO_MONETIZACAO_E_SEGURANCA.md`, não suas evidências de testes. Valores do Kofre abaixo são hipóteses para validação, não vendas ou margens medidas.

## Empresa e recebimento

Decisão do responsável: vender no Brasil e no exterior pelo CNPJ existente da **Delphos**, usando **Kofre / kofre.dev** como marca do produto. O responsável informou o nome empresarial **S. A. DE ARAUJO - ME**, o nome de divulgação **DELPHOS AUTOMAÇÃO** e o contato **kofredev@delphosautomacao.com.br** para suporte, privacidade e propostas Enterprise. A identificação empresarial fornecida e o contato foram incluídos nas páginas comerciais locais. Enquadramento fiscal ainda precisa ser confirmado antes da abertura do checkout. Não armazenar documentos ou credenciais de pagamento neste contexto portátil.

Recomendação: manter conta PJ brasileira da Delphos como base e avaliar **Paddle** para uma única integração global. Uma conta estrangeira não é pré-requisito arquitetural; confirmar moeda, banco elegível, conversão e tarifas de repasse durante o onboarding. A Delphos precisa conferir com seu contador atividade, faturamento e tratamento dos repasses. Merchant of Record cuida dos impostos da venda ao comprador conforme seu contrato; não elimina obrigações brasileiras da empresa.

| Provedor | Evidência consultada | Decisão proposta |
|---|---|---|
| Paddle | Merchant of Record; tarifa padrão 5% + US$ 0,50 por transação; Brasil não consta na lista de fornecedores excluídos | Preferido para avaliar Brasil + exterior, condicionado à aprovação da Delphos e do produto. Negociar tarifa para tickets abaixo de US$ 10. Confirmar meios de pagamento realmente habilitados, inclusive Pix recorrente. |
| Stripe Brasil | Cartão nacional 3,99% + R$ 0,39; internacional mais 2%; Billing 0,7%; Pix somente por convite, 1,19% | Alternativa se Paddle não aprovar ou se operação direta fizer mais sentido. Custos fiscais e produtos adicionais devem entrar na comparação. Não prometer Pix automático só porque aceita Pix. |
| Polar | A lista atual de países elegíveis para vendedores não inclui Brasil | A recomendação antiga de Polar não é adequada à Delphos brasileira sem confirmação expressa de elegibilidade. |

Fontes: [Paddle: preços](https://www.paddle.com/pricing), [Paddle: países](https://www.paddle.com/help/legal/sanctions/which-countries-are-supported-by-paddle), [Stripe Brasil](https://stripe.com/br/pricing), [Polar: países](https://polar.sh/docs/merchant-of-record/supported-countries).

## Mercado e posicionamento

| Referência | Preço exibido na consulta | O que informa a proposta |
|---|---|---|
| Bitwarden Premium | US$ 19,80/ano; equivalente a US$ 1,65/mês | Forte concorrência para um gerenciador pessoal genérico. |
| Bitwarden Teams / Enterprise | US$ 4 / US$ 6 por usuário/mês, cobrança anual | Empresas já encontram permissões, logs e identidade nessa faixa. |
| Passbolt Pro | US$ 4,90/usuário/mês, anual, mínimo 10 usuários | Compartilhamento, SSO, diretório e auditoria são recursos comerciais concretos. |
| Doppler Team | US$ 21/usuário/mês; Enterprise sob proposta | Referência de gestão de segredos para desenvolvimento, com RBAC, identidade, rotação e logs. Não é equivalente funcional ao Kofre atual. |

Fontes: [Bitwarden](https://bitwarden.com/pricing/), [Passbolt](https://www.passbolt.com/pricing/pro), [Doppler](https://www.doppler.com/pricing).

Posicionamento proposto: cofre local para desenvolvedores que trabalham no terminal, com nuvem gerenciada opcional. Demonstrar CLI/TUI, importação e uso de segredos no fluxo de desenvolvimento. Não anunciar inviolabilidade, certificações, auditoria independente, latência ou disponibilidade sem evidência. Criptografia e segurança básica permanecem no plano gratuito.

## Planos

| Plano | Brasil | Internacional | Oferta |
|---|---|---|---|
| Community | Gratuito | Gratuito | Cofre local, CLI/TUI e importação existentes. Sem obrigação de contratar nuvem para acessar o arquivo local. |
| Pro mensal | R$ 19,90/mês | US$ 3,99/mês | Nuvem gerenciada e integração Telegram existentes, após fechar cobrança e direitos de acesso. |
| Pro anual | R$ 199/ano | US$ 39/ano | Mesmo produto; destacar o total anual e renovação, nunca apresentar equivalente mensal como cobrança mensal. |
| Enterprise | Sob proposta | Sob proposta | Programa de desenvolvimento com empresas; não disponível como pacote pronto. Hipótese interna inicial: R$ 49 ou US$ 9 por usuário/mês, mínimo 10, reajustada após escopo e custo de suporte. |

O preço anual brasileiro desconta 16,7% sobre 12 mensalidades; o internacional, aproximadamente 18,5%. BRL e USD são preços regionais, não conversões cambiais prometidas. Não oferecer lifetime para um serviço com custo recorrente.

Na tarifa padrão Paddle, US$ 3,99 deixam aproximadamente US$ 3,29 após apenas a tarifa (17,5%); US$ 39 deixam US$ 36,55 (6,3%). A conta simplificada não inclui impostos, câmbio, repasses, estornos, infraestrutura, atendimento ou aquisição. Não extrapolar margem de 90% nem custo de 1.000 usuários sem medir tamanho de anexos, frequência de sync, transferência e suporte. Modelo: receita reconhecida menos taxas, impostos, reembolsos, custo variável e parcela de custos fixos. Receita anual recebida não é MRR integral do mês.

Expansões Pro propostas, ainda não implementadas: histórico de versões com restauração validada, gestão/revogação de dispositivos, painel de atividade de sync e suporte com prazo definido. O acesso à versão local e a exportação não devem depender de pagamento. Aplicativo móvel e extensão não são benefícios entregues hoje.

## Enterprise e auditoria

Prioridade de produto: organizações isoladas, identidades individuais, cofres compartilhados com distribuição de chaves, permissões por função, remoção de membros, logs de administração e acesso remoto, exportação de eventos e faturamento por assento. Depois: SSO OIDC/SAML, SCIM, políticas, armazenamento dedicado e SLA baseado em operação medida. Usar identidade individual, nunca uma licença/token compartilhado para toda a empresa.

Há duas entregas diferentes: **trilha de auditoria do uso** (quem realizou qual ação, quando e com qual resultado) e **auditoria independente de segurança do produto**. Empresas podem exigir ambas; um log não é um pentest e usar uma infraestrutura certificada não certifica o Kofre.

Eventos devem guardar identificadores e metadados mínimos, sem senha, token, conteúdo do cofre ou nomes sensíveis. Definir retenção contratual, acesso aos logs, exportação, integridade e destino SIEM. Um cliente que funciona offline não prova todas as leituras locais ao servidor; não prometer rastreabilidade completa ou revogação de cópias já exportadas. Cadeia de hashes sozinha não protege contra reescrita por administrador: precisa de armazenamento independente/imutável ou ancoragem verificável. SSO autentica a identidade, não resolve sozinho o desbloqueio criptográfico.

Antes de vender Enterprise como pronto: testar isolamento entre organizações e papéis, retirada de membro e distribuição de chaves, definir recuperação sem quebrar o modelo criptográfico, concluir revisão externa, preparar DPA/contrato e medir recuperação e disponibilidade. Onboarding dedicado pode ser cobrado separadamente, com escopo e esforço claros.

## Venda automática: estado real e desenho

**Estado observado no código:** `POST /v1/auth/provision` emite token Pro sem pagamento. `resolveUserIDFromToken` aceita o prefixo/formato e deriva identidade, sem consultar emissão, assinatura ou vencimento. O cliente considera Pro a configuração CloudEnabled com token. Há piloto de sincronização; não há sistema de assinatura paga. A landing anterior encaminhava ao Telegram, não ao checkout. Essa falha é de controle comercial: não demonstra leitura do cofre de outro usuário.

Ficha: problema: vender nuvem sem direitos verificados; evidência: provisionamento e resolução de token acima; invariante: somente direitos persistidos e válidos autorizam serviço pago, mantendo identidade/dados de clientes atuais e acesso local; hipótese: autenticação e plano foram tratados como a mesma configuração; solução: registro persistente de clientes/assinaturas/dispositivos e webhooks autenticados, com migração explícita do piloto; previsão: pagamento confirmado concede Pro e cancelamento encerra direitos no prazo contratado; risco: bloquear clientes existentes ou mudar a identidade derivada que aponta para seus objetos; oráculo: testes de assinatura falsa/duplicada/fora de ordem, renovação, cancelamento, isolamento e migração com fixtures; gate: nenhum novo token pago nasce pelo endpoint piloto, todos os cenários passam em sandbox e smoke real antes de habilitar cobrança.

Fluxo:

1. Conta Delphos aprovada; produtos, preços regionais e URLs de retorno configurados no provedor.
2. Checkout hospedado, sem armazenar cartão no Kofre. Vincular transação a identidade autenticada no servidor; não confiar em preço/plano enviados pelo navegador.
3. Webhook verifica assinatura sobre corpo original, aceita apenas eventos esperados e grava evento + efeito de forma idempotente. Tratar duplicidade, reordenação, retries e reconciliação com o provedor. Nunca ativar pelo redirecionamento de sucesso.
4. Registrar status, plano, período pago, IDs do provedor e vínculo de cliente. Separar autenticação do direito ao Pro. Ativação por código curto com validade/uso único ou sessão autenticada; não publicar token permanente na URL ou em logs.
5. Migrar contas piloto mantendo o identificador do cofre e a posse comprovada. Não converter qualquer token `kfr_` inventado em licença legada. Definir com o responsável a duração do piloto antes de aplicar bloqueios.
6. Portal para renovar/cancelar, cobrança de falhas e e-mails transacionais. Não utilizar o Telegram como comprovante de pagamento ou único canal de recuperação comercial.
7. Cancelamento desativa apenas recursos pagos na data acordada. Definir janela de download/exportação, retenção e exclusão de nuvem antes do checkout; não excluir cofre como consequência direta de webhook.

Dependências externas pendentes: onboarding Paddle, conta de recebimento, credenciais sandbox/produção inseridas por canal seguro, validação fiscal e políticas comerciais. Razão social e contato oficial foram informados; funcionamento da caixa de e-mail não foi testado. Não há checkout real, integração Paddle ou migração implantada por esta proposta.

## Landing e lançamento

PT-BR e EN com URLs próprias, canonical/hreflang, Community/Pro/Enterprise, preços mensais e anuais explícitos, FAQ e limites da segurança. Community instala Windows x64 atual; não anunciar builds Linux/macOS disponíveis antes de publicá-los. Pro permanece em preparação comercial e Enterprise em planejamento, sem botão fingindo comprar.

Pendências para liberar venda: completar informações legais necessárias da operadora, termos de contratação/cancelamento/reembolso, aviso de privacidade (inclusive metadados, provedores e contato), suporte real, checkout/portal funcional, direitos de acesso testados e revisão da migração do piloto. Materiais jurídicos devem ser revisados conforme mercados atendidos; não inventar endereço, certificado ou prazo contratual.

Validação desta etapa: testes Go do backend, teste de rotas PT/EN e redirecionamentos, race do pacote server, vet, compilação, IDs/âncoras/hreflang e sintaxe dos scripts passaram. A revisão visual ficou pendente: o runtime de navegador retornou nenhum navegador disponível. As páginas foram alteradas apenas localmente, sem deploy nesta etapa. Nenhuma compra ou e-mail real foi enviado.

Validação comercial proposta: primeiras 20 entrevistas com desenvolvedores, 30 clientes Pro pagantes e 3 empresas parceiras. Medir visita → checkout → pagamento → primeira sincronização, tempo de ativação, cancelamentos, reembolsos, chamados e margem por cliente. Coletar apenas telemetria comercial necessária e declarada, sem conteúdo de segredos. Revisar preços após 60 dias de dados, não prometer demanda só com pesquisa de concorrentes.
