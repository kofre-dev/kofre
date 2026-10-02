package crypto

import (
	"bytes"
	"errors"
	"testing"
)

func TestSealedBufferLifecycle(t *testing.T) {
	for _, length := range []int{0, 1, 15, 16, 17, 4096} {
		plain := bytes.Repeat([]byte{'x'}, length)
		sealed, err := SealMemory(plain)
		if err != nil {
			t.Fatal(err)
		}
		var borrowed []byte
		failure := errors.New("erro do consumidor")
		if err := sealed.WithBytes(func(value []byte) error {
			if !bytes.Equal(value, plain) {
				t.Fatal("valor decifrado divergente")
			}
			borrowed = value
			return failure
		}); !errors.Is(err, failure) {
			t.Fatal("erro do consumidor foi perdido")
		}
		if !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
			t.Fatal("buffer temporario nao foi apagado")
		}
		if err := sealed.WithBytes(func(value []byte) error {
			if !bytes.Equal(value, plain) {
				t.Fatal("callback alterou segredo persistente")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sealed.Close()
		sealed.Close()
		if err := sealed.WithBytes(func([]byte) error { t.Fatal("segredo encerrado foi utilizado"); return nil }); err == nil {
			t.Fatal("faltou erro apos Close")
		}
	}
}
