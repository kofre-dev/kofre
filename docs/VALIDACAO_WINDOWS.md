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

O teste do desinstalador também falhou no runner, sem mostrar a causa capturada
pelo script. O teste agora inclui seu log temporário na falha. A remoção continua
restrita a executável fictício e chaves de registro de teste; cofres e backups
devem permanecer intactos. A causa depende da saída detalhada, sem suposição de
que seja falha de permissão, caminho ou PowerShell.
