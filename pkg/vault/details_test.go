package vault

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNotasAnexosProtegidosPreservamConteudoERevogamClones(t *testing.T) {
	v := NewManaged()
	defer v.Close()
	note := []byte("Nota fictícia com acentos, \"aspas\", quebra\n e 🔐")
	attachment := []byte{0, 255, 42, 10, 0, 31, 128}
	entry, err := v.AddEntry(SecretEntry{ID: "fixture", Title: "Fixture", Notes: string(note), Attachments: []Attachment{{Filename: "privada.bin", Data: attachment, Size: 999}}})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Notes != "" || entry.Attachments[0].Data != nil || entry.Attachments[0].Size != int64(len(attachment)) {
		t.Fatal("detalhes não foram protegidos ou tamanho não foi normalizado")
	}
	if _, err := json.Marshal(entry); err == nil {
		t.Fatal("serialização incidental da entrada não protegeu notas/anexos")
	}
	if _, err := json.Marshal(entry.Attachments[0]); err == nil {
		t.Fatal("serialização incidental do anexo descartou silenciosamente seu conteúdo")
	}
	for _, clone := range append(v.Entries(), v.Search("acentos", "")...) {
		if !clone.HasNotes() || !clone.Attachments[0].HasData() {
			t.Fatal("metadados perderam conteúdo")
		}
		assertNotesAttachment(t, clone, note, attachment)
	}
	key, salt := make([]byte, 32), make([]byte, 16)
	packed, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	s, payload, err := UnpackHeader(packed)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := DecryptAndLoad(payload, key, s)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	assertNotesAttachment(t, reloaded.Entries()[0], note, attachment)
	exported, err := v.ExportarEntrada(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	var independent struct {
		Entries []struct {
			Notes       string
			Attachments []Attachment
		}
	}
	if err := json.Unmarshal(exported, &independent); err != nil {
		t.Fatal(err)
	}
	if independent.Entries[0].Notes != string(note) || !bytes.Equal(independent.Entries[0].Attachments[0].Data, attachment) {
		t.Fatal("exportação perdeu detalhes")
	}
	entry.Title = "Editada"
	if err := v.UpdateEntry(entry); err != nil {
		t.Fatal(err)
	}
	if err := entry.WithNotes(func([]byte) error { return nil }); err == nil {
		t.Fatal("notas antigas não foram revogadas")
	}
	if err := entry.Attachments[0].WithData(func([]byte) error { return nil }); err == nil {
		t.Fatal("anexo antigo não foi revogado")
	}
	fresh, err := v.GetEntry(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertNotesAttachment(t, fresh, note, attachment)
	v.Close()
	if err := fresh.WithNotes(func([]byte) error { return nil }); err == nil {
		t.Fatal("Close não revogou notas")
	}
	if err := fresh.Attachments[0].WithData(func([]byte) error { return nil }); err == nil {
		t.Fatal("Close não revogou anexo")
	}
}

func assertNotesAttachment(t *testing.T, entry SecretEntry, note, attachment []byte) {
	t.Helper()
	var heldNotes, heldData []byte
	if err := entry.WithNotes(func(value []byte) error {
		heldNotes = value
		if !bytes.Equal(value, note) {
			t.Fatal("nota alterada")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := entry.Attachments[0].WithData(func(value []byte) error {
		heldData = value
		if !bytes.Equal(value, attachment) {
			t.Fatal("anexo alterado")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(heldNotes, make([]byte, len(heldNotes))) || !bytes.Equal(heldData, make([]byte, len(heldData))) {
		t.Fatal("callback reteve buffer em texto simples")
	}
}

func TestApagarNotasNaoReutilizaHandleAntigo(t *testing.T) {
	v := NewManaged()
	defer v.Close()
	entry, err := v.AddEntry(SecretEntry{Title: "Fixture", Notes: "Nota fictícia"})
	if err != nil {
		t.Fatal(err)
	}
	if err := entry.SetNotes(nil); err != nil {
		t.Fatal(err)
	}
	defer entry.CloseNotes()
	if err := v.UpdateEntry(entry); err != nil {
		t.Fatal(err)
	}
	if got, err := v.GetEntry(entry.ID); err != nil || got.HasNotes() {
		t.Fatal("nota excluída foi preservada")
	}
}

func TestDecoderProtegidoRecusaDetalhesMalformados(t *testing.T) {
	for _, raw := range []string{
		`{"entries":[{"notes":123}]}`,
		`{"entries":[{"attachments":[{"data":"#inválido"}]}]}`,
		`{"entries":[{"attachments":[{"data":{}}]}]}`,
	} {
		if _, err := decodeVaultProtected([]byte(raw)); err == nil {
			t.Fatal("detalhes malformados aceitos")
		}
	}
}
