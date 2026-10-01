package web

import (
	_ "embed"
)

// IndexHTML contém os bytes da landing page oficial embutida no binário do servidor
//
//go:embed index.html
var IndexHTML []byte

// DocsHTML contém os bytes da página de documentação técnica
//
//go:embed docs.html
var DocsHTML []byte

