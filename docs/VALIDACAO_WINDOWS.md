# Validação no runner Windows

Problema: o teste de DACL presumiu ausência de privilégios de depuração no
processo executor. O runner Windows abriu o handle que o teste esperava negar;
o mesmo teste passou no PC local. A Microsoft documenta que SeDebugPrivilege
habilitado ignora o descritor de segurança em OpenProcess.

Invariante: o teste deve verificar negação por DACL a um chamador sem privilégio
de depuração, sem ignorar falhas nem alterar os privilégios do runner inteiro.
Solução: fixar a goroutine à thread, usar cópia do token por ImpersonateSelf,
desabilitar privilégios nessa cópia e reverter ao terminar. A negação precisa
ser exatamente ERROR_ACCESS_DENIED. A proteção não promete bloquear administrador
com depuração habilitada nem código no kernel.

Fonte: https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-openprocess

O log do desinstalador revelou `Destino invalido`. Reprodu??o local com alias
8.3 confirmou a causa: GetFullPath expande o alias, mas Join-Path o conserva.
A compara??o passa a normalizar ambos os lados. O teste executa a desinstala??o
com caminho longo e com alias 8.3, usando apenas execut?vel fict?cio e chaves
isoladas de registro. A variante 8.3 falhou antes e passou ap?s a corre??o;
a su?te do instalador passou com race. Cofre, configura??es, backups e o tipo
EXPAND_SZ do PATH permanecem preservados. Volumes sem nomes curtos dispensam
somente a variante 8.3; o teste normal continua obrigat?rio.
