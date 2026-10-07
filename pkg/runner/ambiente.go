package runner

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"

	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

var nomeVariavel = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// ValidarSelecao impede abrir o cofre quando a operação não declara seu alcance.
func ValidarSelecao(entrada string, campos []string) error {
	if strings.TrimSpace(entrada) == "" || len(campos) == 0 {
		return errors.New("informe --entry com ID ou título exato e --fields com os campos necessários")
	}
	vistos := make(map[string]bool)
	for _, campo := range campos {
		if strings.TrimSpace(campo) == "" || vistos[campo] {
			return errors.New("a seleção contém campo vazio ou repetido")
		}
		vistos[campo] = true
	}
	return nil
}

func credencialSelecionada(v *vault.ManagedVault, seletor string) (vault.SecretEntry, error) {
	// Um ID tem prioridade sobre um título que coincidentemente use o mesmo texto.
	if entrada, err := v.GetEntry(seletor); err == nil {
		return entrada, nil
	}
	var encontrada vault.SecretEntry
	quantidade := 0
	for _, entrada := range v.Entries() {
		if entrada.Title == seletor {
			encontrada = entrada
			quantidade++
		}
	}
	if quantidade == 0 {
		return vault.SecretEntry{}, errors.New("nenhuma credencial corresponde ao ID ou título exato informado")
	}
	if quantidade != 1 {
		return vault.SecretEntry{}, errors.New("há títulos iguais; selecione a credencial pelo ID")
	}
	return encontrada, nil
}

// valoresSelecionados mantém buffers mutáveis até a fronteira os/exec. Notas,
// anexos e campos não solicitados nunca são exportados implicitamente.
func valoresSelecionados(v *vault.ManagedVault, seletor string, campos []string) (map[string][]byte, error) {
	if err := ValidarSelecao(seletor, campos); err != nil {
		return nil, err
	}
	entrada, err := credencialSelecionada(v, seletor)
	if err != nil {
		return nil, err
	}
	valores := make(map[string][]byte)
	sucesso := false
	defer func() {
		if !sucesso {
			limparValores(valores)
		}
	}()
	for _, nome := range campos {
		var campo *vault.Field
		for i := range entrada.Fields {
			if entrada.Fields[i].Name == nome {
				if campo != nil {
					return nil, errors.New("a credencial contém nomes de campo repetidos")
				}
				campo = &entrada.Fields[i]
			}
		}
		if campo == nil {
			return nil, fmt.Errorf("campo solicitado não encontrado: %q", nome)
		}
		nomeEnv := strings.ToUpper(nome)
		if !nomeVariavel.MatchString(nomeEnv) {
			// Preserve '_' ao normalizar nomes livres como "API - token".
			nomeEnv = vault.SanitizeEnvKey(strings.ReplaceAll(nome, "_", " "))
		}
		if !nomeVariavel.MatchString(nomeEnv) || variavelDeDesbloqueio(nomeEnv) {
			return nil, errors.New("campo não pode ser usado como variável de ambiente")
		}
		if _, repetido := valores[nomeEnv]; repetido {
			return nil, errors.New("campos selecionados geram a mesma variável de ambiente")
		}
		if err := campo.WithValue(func(raw []byte) error {
			if bytes.IndexByte(raw, 0) >= 0 {
				return errors.New("campo contém byte nulo incompatível com variável de ambiente")
			}
			valores[nomeEnv] = bytes.Clone(raw)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sucesso = true
	return valores, nil
}

func limparValores(valores map[string][]byte) {
	for nome, valor := range valores {
		mycrypto.ZeroBytes(valor)
		delete(valores, nome)
	}
}

func variavelDeDesbloqueio(nome string) bool {
	return strings.EqualFold(nome, "KOFRE_PIN") || strings.EqualFold(nome, "MYCOFRE_PIN")
}

func nomeAmbiente(nome string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(nome)
	}
	return nome
}

func ambienteFilho(herdado []string, valores map[string][]byte) []string {
	ambiente := make([]string, 0, len(herdado)+len(valores))
	for _, item := range herdado {
		nome, _, ok := strings.Cut(item, "=")
		if !ok || variavelDeDesbloqueio(nome) {
			continue
		}
		if _, substituido := valores[nomeAmbiente(nome)]; substituido {
			continue
		}
		ambiente = append(ambiente, item)
	}
	nomes := make([]string, 0, len(valores))
	for nome := range valores {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	for _, nome := range nomes {
		ambiente = append(ambiente, nome+"="+string(valores[nome]))
	}
	return ambiente
}

func prepararAmbiente(path, entrada string, campos []string) ([]string, int, error) {
	if err := ValidarSelecao(entrada, campos); err != nil {
		return nil, 0, err
	}
	v, err := UnlockVault(path)
	if err != nil {
		return nil, 0, err
	}
	defer v.Close()
	valores, err := valoresSelecionados(v, entrada, campos)
	if err != nil {
		return nil, 0, err
	}
	defer limparValores(valores)
	return ambienteFilho(os.Environ(), valores), len(valores), nil
}

// A API os/exec exige strings. Soltamos as referências após Start; não é uma
// garantia de sobrescrever strings imutáveis nem a memória do processo filho.
func limparAmbiente(ambiente []string) {
	for i := range ambiente {
		ambiente[i] = ""
	}
}
