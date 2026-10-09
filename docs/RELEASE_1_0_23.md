# Versão 1.0.23

Mantém o cadastro de bancos de dados da 1.0.22 e exige Go 1.26.9 para os
builds. O gate de vulnerabilidades do CI encontrou nove vulnerabilidades
alcançáveis da biblioteca padrão ao verificar o build com Go 1.26.8.
As correções da revisão 1.26.9 foram publicadas em 8 de outubro de 2026:
[histórico oficial do Go](https://go.dev/doc/devel/release#go1.26.0).

Problema: gate de vulnerabilidades reprovado após a publicação 1.0.22;
evidência: `govulncheck` no CI e local apontou biblioteca padrão Go 1.26.8;
invariante: preservar credenciais e protocolo de cifra, não sobrescrever
releases assinadas existentes; causa: toolchain anterior às correções de
segurança publicadas no mesmo dia; referência: histórico oficial do Go e
diagnóstico do `govulncheck`; solução: elevar a revisão mínima para 1.26.9
e recompilar as cinco plataformas como 1.0.23; previsão: gate aprovado sem
vulnerabilidades alcançáveis reportadas; riscos/não objetivos: o scanner não
prova ausência de todas as falhas, sem mudança no formato do cofre;
oráculo: homologação, scanner, assinatura, download público e atualização
real em instalação isolada; gate: publicar builds novos somente após os
checks locais aprovados e conferir o CI remoto.
