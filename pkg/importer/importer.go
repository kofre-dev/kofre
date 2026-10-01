package importer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"kofre/pkg/vault"
)

// Limites de segurança e integridade
const (
	MaxFileSize    = 10 * 1024 * 1024 // 10 MB máximo por arquivo individual
	MaxTitleLength = 100              // Trunca títulos excessivos
	MaxFieldLength = 100 * 1024       // 100 KB por campo (suporta chaves SSH e certificados)
	MaxNotesLength = 500 * 1024       // 500 KB de notas
)

var (
	// Detectores de chaves comuns de desenvolvedor
	reOpenAI    = regexp.MustCompile(`sk-[a-zA-Z0-9_\-]{20,}`)
	reGitHub    = regexp.MustCompile(`(ghp_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{50,})`)
	reAWSAccess = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	reAWSSecret = regexp.MustCompile(`[0-9a-zA-Z/+=]{40}`)
	reURL       = regexp.MustCompile(`https?://[^\s]+`)
	reEmail     = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

// Extensões de código-fonte, binários ou caches que NUNCA devem ser importados como credenciais
var ignoredExtensions = map[string]bool{
	".exe": true, ".dll": true, ".bin": true, ".iso": true, ".sys": true,
	".zip": true, ".rar": true, ".7z": true, ".tar": true, ".gz": true,
	".php": true, ".vb": true, ".cs": true, ".sql": true, ".js": true,
	".ts": true, ".py": true, ".rb": true, ".go": true, ".java": true,
	".c": true, ".cpp": true, ".h": true, ".html": true, ".css": true,
	".xml": true, ".dylib": true, ".so": true, ".class": true, ".jar": true,
	".obj": true, ".o": true, ".bak": true, ".tmp": true,
}

// ParseTarget analisa um arquivo individual OU uma pasta inteira recursivamente
func ParseTarget(targetPath string) ([]vault.SecretEntry, error) {
	cleanPath := filepath.Clean(targetPath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("caminho não encontrado: %w", err)
	}

	if !info.IsDir() {
		return parseSingleFile(cleanPath)
	}

	var allEntries []vault.SecretEntry
	err = filepath.Walk(cleanPath, func(path string, f os.FileInfo, err error) error {
		if err != nil || f.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ignoredExtensions[ext] {
			return nil
		}

		// Ignora arquivos ocultos de sistema (.DS_Store, .git, etc.)
		if strings.HasPrefix(filepath.Base(path), ".") && ext != ".env" {
			return nil
		}

		entries, err := parseSingleFile(path)
		if err == nil {
			allEntries = append(allEntries, entries...)
		}
		return nil
	})

	return allEntries, err
}

func parseSingleFile(filePath string) ([]vault.SecretEntry, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("arquivo excede o limite máximo de segurança de 10 MB (%d bytes)", info.Size())
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	// 1. Arquivos CSV estruturados (Bitwarden, 1Password, Chrome, Kofre)
	if ext == ".csv" {
		return parseCSVFile(filePath)
	}

	// 2. Arquivos JSON ou JSONL estruturados
	if ext == ".json" || ext == ".jsonl" {
		return parseJSONFile(filePath)
	}

	// 3. Imagens e prints de segurança (.png, .jpg, .jpeg)
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return []vault.SecretEntry{
			{
				Title:    sanitizeString(fmt.Sprintf("Imagem/Cartão: %s", baseName), MaxTitleLength),
				Category: vault.CategoryAuth,
				Attachments: []vault.Attachment{
					{
						Filename: filepath.Base(filePath),
						Data:     data,
						Size:     int64(len(data)),
					},
				},
				Notes: fmt.Sprintf("Importado com segurança de %s", filepath.Base(filePath)),
			},
		}, nil
	}

	// 4. Certificados PFX / P12 (binários criptográficos)
	if ext == ".pfx" || ext == ".p12" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return []vault.SecretEntry{
			{
				Title:    sanitizeString(fmt.Sprintf("Certificado: %s", baseName), MaxTitleLength),
				Category: vault.CategoryCertificate,
				Attachments: []vault.Attachment{
					{
						Filename: filepath.Base(filePath),
						Data:     data,
						Size:     int64(len(data)),
					},
				},
				Notes: fmt.Sprintf("Importado de %s", filepath.Base(filePath)),
			},
		}, nil
	}

	// 5. Chaves PEM / PPK / KEY
	if ext == ".pem" || ext == ".ppk" || ext == ".key" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		cat := vault.CategorySSHKey
		if strings.Contains(strings.ToLower(filePath), "cert") || ext == ".pem" {
			cat = vault.CategoryCertificate
		}
		return []vault.SecretEntry{
			{
				Title:    sanitizeString(fmt.Sprintf("%s (%s)", baseName, strings.ToUpper(strings.TrimPrefix(ext, "."))), MaxTitleLength),
				Category: cat,
				Fields: []vault.Field{
					{Name: "Payload", Value: sanitizeString(string(data), MaxFieldLength), Protected: true},
				},
				Notes: fmt.Sprintf("Importado de %s", filepath.Base(filePath)),
			},
		}, nil
	}

	// 6. Arquivos de texto (.txt, .md, .env, scripts de config)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	rawContent := string(data)

	// Se contiver delimitadores claros de múltiplas seções (===, ###, [seção])
	if hasMultipleSections(rawContent) {
		entries, err := ParseMessyTxt(filePath)
		if err == nil && len(entries) > 0 {
			return entries, nil
		}
	}

	// Trata o arquivo inteiro como uma única credencial coesa
	singleEntry := parseEntireFileAsEntry(rawContent, baseName)
	if len(singleEntry.Fields) == 0 && singleEntry.Notes == "" {
		return nil, nil // Ignora arquivos em branco
	}
	return []vault.SecretEntry{singleEntry}, nil
}

func hasMultipleSections(content string) bool {
	lines := strings.Split(content, "\n")
	headerCount := 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if (strings.HasPrefix(trimmed, "===") && strings.HasSuffix(trimmed, "===") && len(trimmed) > 6) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && len(trimmed) > 2) ||
			(strings.HasPrefix(trimmed, "### ") && len(trimmed) > 4) {
			headerCount++
		}
	}
	return headerCount > 1
}

func parseEntireFileAsEntry(content, baseName string) vault.SecretEntry {
	entry := parseSingleBlock(content, 1)
	entry.Title = sanitizeString(baseName, MaxTitleLength)
	entry.Category = normalizeCategory(baseName)
	return entry
}

// ParseMessyTxt analisa um arquivo de texto desformatado e extrai credenciais heurísticas
func ParseMessyTxt(filePath string) ([]vault.SecretEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir arquivo: %w", err)
	}
	defer file.Close()

	var blocks []string
	var currentBlock []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Separador de bloco: linha em branco ou divisores (---, ===, ***)
		if isSeparatorLine(trimmed) {
			if len(currentBlock) > 0 {
				blocks = append(blocks, strings.Join(currentBlock, "\n"))
				currentBlock = nil
			}
			continue
		}

		currentBlock = append(currentBlock, line)
	}

	if len(currentBlock) > 0 {
		blocks = append(blocks, strings.Join(currentBlock, "\n"))
	}

	var entries []vault.SecretEntry
	for i, b := range blocks {
		entry := parseSingleBlock(b, i+1)
		// Filtra blocos vazios
		if len(entry.Fields) > 0 || entry.Notes != "" {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func parseSingleBlock(raw string, index int) vault.SecretEntry {
	lines := strings.Split(raw, "\n")
	firstLine := strings.TrimSpace(lines[0])

	title := cleanHeader(firstLine)
	if title == "" {
		title = fmt.Sprintf("Credencial %d", index)
	}

	category := vault.CategoryPassword
	fields := make([]vault.Field, 0)
	var notesLines []string
	inExplicitNotes := false

	// Se contiver chave SSH ou Certificado PEM
	if strings.Contains(raw, "-----BEGIN") {
		if strings.Contains(raw, "PRIVATE KEY") {
			category = vault.CategorySSHKey
		} else {
			category = vault.CategoryCertificate
		}
		fields = append(fields, vault.Field{
			Name:      "Payload",
			Value:     sanitizeString(raw, MaxFieldLength),
			Protected: true,
		})
		return vault.SecretEntry{
			Title:    sanitizeString(title, MaxTitleLength),
			Category: category,
			Fields:   fields,
		}
	}

	// Detecta chaves de API diretas
	if openAIKey := reOpenAI.FindString(raw); openAIKey != "" {
		category = vault.CategoryToken
		fields = append(fields, vault.Field{Name: "OPENAI_API_KEY", Value: openAIKey, Protected: true})
	}
	if ghKey := reGitHub.FindString(raw); ghKey != "" {
		category = vault.CategoryToken
		fields = append(fields, vault.Field{Name: "GITHUB_TOKEN", Value: ghKey, Protected: true})
	}
	if awsKey := reAWSAccess.FindString(raw); awsKey != "" {
		category = vault.CategoryToken
		fields = append(fields, vault.Field{Name: "AWS_ACCESS_KEY_ID", Value: awsKey, Protected: false})
	}

	hasStructuredFields := len(fields) > 0
	skipNext := false

	for i := 0; i < len(lines); i++ {
		if skipNext {
			skipNext = false
			continue
		}

		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}

		// Se a primeira linha foi usada como título, não a processa como campo
		if i == 0 && (strings.HasPrefix(trimmed, "===") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "###")) {
			continue
		}

		// Se estamos dentro de um bloco de notas explícito
		if inExplicitNotes {
			notesLines = append(notesLines, trimmed)
			continue
		}

		lower := strings.ToLower(trimmed)

		// Declaração explícita de notas
		if strings.HasPrefix(lower, "notas:") || strings.HasPrefix(lower, "notes:") || strings.HasPrefix(lower, "obs:") {
			inExplicitNotes = true
			val := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
			if val != "" {
				notesLines = append(notesLines, val)
			}
			continue
		}

		// Declaração explícita de categoria
		if strings.HasPrefix(lower, "categoria:") || strings.HasPrefix(lower, "category:") || strings.HasPrefix(lower, "tipo:") {
			catVal := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
			category = normalizeCategory(catVal)
			continue
		}

		// Detecta padrão de email solto na linha (ex: usuario@email.com)
		if reEmail.MatchString(trimmed) {
			fields = append(fields, vault.Field{Name: "Email", Value: sanitizeString(trimmed, 500), Protected: false})
			hasStructuredFields = true
			category = vault.CategoryPassword

			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if nextLine != "" && !reEmail.MatchString(nextLine) && !strings.HasPrefix(nextLine, "http") {
					fields = append(fields, vault.Field{Name: "Senha", Value: sanitizeString(nextLine, MaxFieldLength), Protected: true})
					skipNext = true
					continue
				}
			}
			continue
		}

		// Procura delimitador : ou =
		delimIdx := strings.IndexAny(trimmed, ":=")
		if delimIdx > 0 && delimIdx < len(trimmed)-1 {
			k := strings.TrimSpace(trimmed[:delimIdx])
			v := strings.TrimSpace(trimmed[delimIdx+1:])

			if k != "" && v != "" {
				isProtected := isPasswordField(k)
				fields = append(fields, vault.Field{
					Name:      sanitizeString(k, 50),
					Value:     sanitizeString(v, MaxFieldLength),
					Protected: isProtected,
				})
				hasStructuredFields = true
				continue
			}
		}

		// Se tiver uma URL solta
		if url := reURL.FindString(trimmed); url != "" {
			fields = append(fields, vault.Field{Name: "URL", Value: sanitizeString(url, 2000), Protected: false})
			hasStructuredFields = true
			continue
		}

		// Linhas soltas só entram nas notas se o bloco não contiver campos de credenciais
		if !hasStructuredFields {
			notesLines = append(notesLines, trimmed)
		}
	}

	// Se não encontrou campos estruturados mas tem notas, classifica como Nota segura
	if !hasStructuredFields && len(notesLines) > 0 {
		category = vault.CategoryNote
	}

	return vault.SecretEntry{
		Title:    sanitizeString(title, MaxTitleLength),
		Category: category,
		Fields:   fields,
		Notes:    sanitizeString(strings.Join(notesLines, "\n"), MaxNotesLength),
	}
}

func parseCSVFile(filePath string) ([]vault.SecretEntry, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	delim := detectCSVDelimiter(filePath)
	reader := csv.NewReader(f)
	reader.Comma = delim
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao processar CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("arquivo CSV vazio ou sem dados")
	}

	header := records[0]
	colTitle := -1
	colCat := -1
	colUser := -1
	colPass := -1
	colURL := -1
	colNotes := -1

	for i, h := range header {
		clean := strings.ToLower(strings.TrimSpace(h))
		clean = strings.Trim(clean, "\ufeff\"' ")
		switch clean {
		case "title", "name", "titulo", "nome", "servico", "service", "system":
			colTitle = i
		case "category", "categoria", "type", "tipo":
			colCat = i
		case "username", "user", "usuario", "login", "email", "e-mail":
			colUser = i
		case "password", "senha", "pass", "secret", "token", "chave":
			colPass = i
		case "url", "site", "uri", "link", "endereco", "host":
			colURL = i
		case "notes", "nota", "notas", "comments", "comentarios", "obs", "note":
			colNotes = i
		}
	}

	var entries []vault.SecretEntry
	for rowIdx, row := range records[1:] {
		title := ""
		if colTitle >= 0 && colTitle < len(row) {
			title = sanitizeString(row[colTitle], MaxTitleLength)
		}
		if title == "" {
			if colURL >= 0 && colURL < len(row) {
				title = titleFromURL(row[colURL])
			}
			if title == "" {
				title = fmt.Sprintf("Item %d", rowIdx+1)
			}
		}

		catStr := "password"
		if colCat >= 0 && colCat < len(row) && strings.TrimSpace(row[colCat]) != "" {
			catStr = strings.ToLower(strings.TrimSpace(row[colCat]))
		}
		category := normalizeCategory(catStr)

		var fields []vault.Field
		if colUser >= 0 && colUser < len(row) {
			u := sanitizeString(row[colUser], 500)
			if u != "" {
				fields = append(fields, vault.Field{Name: "Usuario", Value: u, Protected: false})
			}
		}
		if colPass >= 0 && colPass < len(row) {
			p := sanitizeString(row[colPass], MaxFieldLength)
			if p != "" {
				fields = append(fields, vault.Field{Name: "Senha", Value: p, Protected: true})
			}
		}
		if colURL >= 0 && colURL < len(row) {
			u := sanitizeString(row[colURL], 2000)
			if u != "" {
				fields = append(fields, vault.Field{Name: "URL", Value: u, Protected: false})
			}
		}

		notes := ""
		if colNotes >= 0 && colNotes < len(row) {
			notes = sanitizeString(row[colNotes], MaxNotesLength)
		}

		// Adiciona colunas extras que contenham dados
		for i, val := range row {
			if i != colTitle && i != colCat && i != colUser && i != colPass && i != colURL && i != colNotes {
				val = sanitizeString(val, MaxFieldLength)
				if val != "" && i < len(header) {
					colName := sanitizeString(header[i], 50)
					fields = append(fields, vault.Field{Name: colName, Value: val, Protected: isPasswordField(colName)})
				}
			}
		}

		if len(fields) == 0 && notes == "" {
			continue
		}

		entries = append(entries, vault.SecretEntry{
			Title:    title,
			Category: category,
			Fields:   fields,
			Notes:    notes,
		})
	}

	return entries, nil
}

func parseJSONFile(filePath string) ([]vault.SecretEntry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var rawList []map[string]interface{}
	// Suporta formato direto ou embrulhado em items/entries
	if err := json.Unmarshal(data, &rawList); err != nil {
		var wrapped struct {
			Items   []map[string]interface{} `json:"items"`
			Entries []map[string]interface{} `json:"entries"`
		}
		if errWrap := json.Unmarshal(data, &wrapped); errWrap == nil {
			if len(wrapped.Items) > 0 {
				rawList = wrapped.Items
			} else if len(wrapped.Entries) > 0 {
				rawList = wrapped.Entries
			}
		}
	}

	if len(rawList) == 0 {
		// Suporta formato JSON Lines (.jsonl) ou objetos JSON soltos linha a linha
		scanner := bufio.NewScanner(bytes.NewReader(data))
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, MaxFileSize)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
				continue
			}
			var item map[string]interface{}
			if errLine := json.Unmarshal([]byte(line), &item); errLine == nil && len(item) > 0 {
				rawList = append(rawList, item)
			}
		}
	}

	if len(rawList) == 0 {
		return nil, fmt.Errorf("nenhuma credencial identificada no JSON/JSONL")
	}

	var entries []vault.SecretEntry
	for idx, item := range rawList {
		title := ""
		for _, k := range []string{"title", "name", "titulo", "nome", "servico"} {
			if v, ok := item[k].(string); ok && strings.TrimSpace(v) != "" {
				title = sanitizeString(v, MaxTitleLength)
				break
			}
		}
		if title == "" {
			for _, k := range []string{"url", "site", "uri", "link"} {
				if v, ok := item[k].(string); ok && strings.TrimSpace(v) != "" {
					title = titleFromURL(v)
					break
				}
			}
			if title == "" {
				title = fmt.Sprintf("Item %d", idx+1)
			}
		}

		catStr := "password"
		for _, k := range []string{"category", "categoria", "type", "tipo"} {
			if v, ok := item[k].(string); ok && strings.TrimSpace(v) != "" {
				catStr = strings.ToLower(strings.TrimSpace(v))
				break
			}
		}
		category := normalizeCategory(catStr)

		var fields []vault.Field
		for _, k := range []string{"username", "user", "usuario", "login", "email"} {
			if v, ok := item[k].(string); ok && strings.TrimSpace(v) != "" {
				fields = append(fields, vault.Field{Name: "Usuario", Value: sanitizeString(v, 500), Protected: false})
				break
			}
		}
		for _, k := range []string{"password", "senha", "pass", "secret", "token", "chave"} {
			if v, ok := item[k].(string); ok && strings.TrimSpace(v) != "" {
				fields = append(fields, vault.Field{Name: "Senha", Value: sanitizeString(v, MaxFieldLength), Protected: true})
				break
			}
		}
		for _, k := range []string{"url", "site", "uri", "link"} {
			if v, ok := item[k].(string); ok && strings.TrimSpace(v) != "" {
				fields = append(fields, vault.Field{Name: "URL", Value: sanitizeString(v, 2000), Protected: false})
				break
			}
		}

		notes := ""
		for _, k := range []string{"notes", "nota", "notas", "comments", "comentarios", "obs", "note"} {
			if v, ok := item[k].(string); ok {
				notes = sanitizeString(v, MaxNotesLength)
				break
			}
		}

		if len(fields) == 0 && notes == "" {
			continue
		}

		entries = append(entries, vault.SecretEntry{
			Title:    title,
			Category: category,
			Fields:   fields,
			Notes:    notes,
		})
	}

	return entries, nil
}

func detectCSVDelimiter(filePath string) rune {
	f, err := os.Open(filePath)
	if err != nil {
		return ','
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		line := scanner.Text()
		commaCount := strings.Count(line, ",")
		semiCount := strings.Count(line, ";")
		tabCount := strings.Count(line, "\t")

		if semiCount > commaCount && semiCount > tabCount {
			return ';'
		}
		if tabCount > commaCount && tabCount > semiCount {
			return '\t'
		}
	}
	return ','
}

func sanitizeString(s string, maxLen int) string {
	if !utf8.ValidString(s) {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		s = string(v)
	}
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.TrimSpace(s)
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

func cleanHeader(s string) string {
	s = strings.Trim(s, "#=[]-*: ")
	if len(s) > 50 {
		return s[:50] + "..."
	}
	return s
}

func normalizeCategory(s string) vault.Category {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "token") || strings.Contains(s, "api") || strings.Contains(s, "jwt"):
		return vault.CategoryToken
	case strings.Contains(s, "ssh") || strings.Contains(s, "key") || strings.Contains(s, "ppk"):
		return vault.CategorySSHKey
	case strings.Contains(s, "cert") || strings.Contains(s, "pfx") || strings.Contains(s, "p12") || strings.Contains(s, "pem"):
		return vault.CategoryCertificate
	case strings.Contains(s, "auth") || strings.Contains(s, "2fa") || strings.Contains(s, "totp") || strings.Contains(s, "card"):
		return vault.CategoryAuth
	case strings.Contains(s, "note") || strings.Contains(s, "nota") || strings.Contains(s, "anotacao"):
		return vault.CategoryNote
	default:
		return vault.CategoryPassword
	}
}

func isPasswordField(name string) bool {
	n := strings.ToLower(name)
	sensitive := []string{"senha", "password", "pass", "secret", "token", "key", "chave", "pin", "auth", "privada"}
	for _, s := range sensitive {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

func isSeparatorLine(trimmed string) bool {
	if trimmed == "" {
		return true
	}
	if len(trimmed) >= 3 && strings.Trim(trimmed, "-=_*# ") == "" {
		return true
	}
	return false
}

func titleFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err == nil && u.Host != "" {
		host := u.Host
		host = strings.TrimPrefix(host, "www.")
		return host
	}
	return ""
}

