package tui

import (
	"bytes"
	"errors"
	"unicode"
	"unicode/utf8"
)

const minimoCaracteresSenhaMestra = 6

// A política vale ao escolher uma senha, sem impedir a abertura de cofres antigos.
// Espaços externos seguem a normalização da abertura e não contam no mínimo.
func validarNovaSenhaMestra(senha []byte) error {
	senha = bytes.TrimSpace(senha)
	if !utf8.Valid(senha) {
		return errors.New("a senha mestra contém caracteres inválidos")
	}
	if utf8.RuneCount(senha) < minimoCaracteresSenhaMestra {
		return errors.New("a senha mestra deve ter no mínimo 6 caracteres")
	}
	complemento := false
	for len(senha) > 0 {
		r, tamanho := utf8.DecodeRune(senha)
		senha = senha[tamanho:]
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return errors.New("a senha mestra não pode conter caracteres de controle ou invisíveis")
		}
		if unicode.IsLetter(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) || r == ' ' {
			complemento = true
		}
	}
	if !complemento {
		return errors.New("inclua uma letra, símbolo ou espaço interno na senha mestra")
	}
	return nil
}
