package importer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/runner"
	"kofre/pkg/storage"
)

// RunInteractiveImport executa o fluxo interativo de importacao com pre-visualizacao
func RunInteractiveImport(srcTxtPath, vaultPath string, store storage.StorageProvider, autoConfirm bool) error {
	if _, err := os.Stat(srcTxtPath); os.IsNotExist(err) {
		return fmt.Errorf("arquivo de origem nao encontrado: %s", srcTxtPath)
	}

	entries, err := ParseTarget(srcTxtPath)
	if err != nil {
		return fmt.Errorf("falha ao analisar arquivos: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("Nenhum bloco de credencial identificado.")
		return nil
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Kofre 🔐 Pré-visualização da Importação (%d encontradas)     ║\n", len(entries))
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	for i, e := range entries {
		fieldSummary := make([]string, 0)
		for _, f := range e.Fields {
			val := f.Value
			if f.Protected {
				val = "••••••"
			}
			fieldSummary = append(fieldSummary, fmt.Sprintf("%s: %s", f.Name, val))
		}

		summaryStr := strings.Join(fieldSummary, ", ")
		if summaryStr == "" && len(e.Attachments) > 0 {
			summaryStr = fmt.Sprintf("Anexo: %s (%d bytes)", e.Attachments[0].Filename, e.Attachments[0].Size)
		} else if summaryStr == "" && e.Notes != "" {
			lines := strings.Split(e.Notes, "\n")
			summaryStr = fmt.Sprintf("Nota (%d linhas)", len(lines))
		}

		fmt.Printf("  %2d. [%-7s] %-28s → %s\n", i+1, strings.ToUpper(string(e.Category)), e.Title, summaryStr)
	}

	if !autoConfirm {
		fmt.Println()
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Deseja importar todas essas credenciais para o Kofre criptografado? [S/n]: ")
		ans, _ := reader.ReadString('\n')
		ans = strings.TrimSpace(strings.ToLower(ans))

		if ans != "" && ans != "s" && ans != "sim" && ans != "y" && ans != "yes" {
			fmt.Println("Importação cancelada.")
			return nil
		}
	}

	// Desbloqueia o cofre e obtem a chave e salt de sessao
	v, key, salt, err := runner.UnlockVaultWithKey(vaultPath)
	if err != nil {
		return fmt.Errorf("falha ao abrir cofre: %w", err)
	}
	defer mycrypto.ZeroBytes(key)
	defer v.Close()

	for _, e := range entries {
		if _, err := v.AddEntry(e); err != nil {
			return fmt.Errorf("falha ao proteger entrada importada: %w", err)
		}
	}

	// Empacota e salva
	packed, err := v.Pack(key, salt)
	if err != nil {
		return fmt.Errorf("falha ao re-criptografar cofre: %w", err)
	}

	ctx := context.Background()
	if err := store.Save(ctx, packed); err != nil {
		return fmt.Errorf("falha ao gravar cofre no disco: %w", err)
	}

	fmt.Printf("\n✓ %d credenciais foram importadas e criptografadas com sucesso no Kofre!\n", len(entries))
	fmt.Println("Os arquivos originais foram mantidos 100% intactos.")
	fmt.Println("Abra o 'kofre' para navegar e buscar suas credenciais.")
	return nil
}
