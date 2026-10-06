package corporativo

import (
	"encoding/hex"
	"strings"
)

func CredencialContaValida(token string) bool {
	segredo := strings.TrimPrefix(token, "kfr_conta_")
	if segredo != token {
		// Raiz criada no dispositivo ou formato administrativo legado.
		p := strings.Split(segredo, "_")
		if len(p) == 2 && IDValido(p[0]) {
			segredo = p[1]
		} else if len(p) != 1 {
			return false
		}
	} else {
		p := strings.Split(strings.TrimPrefix(token, "kfr_sessao_"), "_")
		if !strings.HasPrefix(token, "kfr_sessao_") || len(p) != 3 || !IDValido(p[0]) || !IDValido(p[1]) {
			return false
		}
		segredo = p[2]
	}
	b, e := hex.DecodeString(segredo)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == segredo
}
