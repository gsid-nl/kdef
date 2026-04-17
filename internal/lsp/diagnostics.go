package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/gsid-nl/kdef/internal/parser"
	"github.com/gsid-nl/kdef/internal/types"
)

// diagnoseFile parses a kdef file and returns LSP diagnostics.
func (s *Server) diagnoseFile(uri string, content string) []protocol.Diagnostic {
	filename := uriToPath(uri)
	basename := filepath.Base(filename)

	var diags hcl.Diagnostics

	switch {
	case basename == "vars.kdef":
		_, diags = parser.ParseVariableFileFromBytes([]byte(content), filename)
	case basename == "root.kdef":
		// Root files have a different schema; just do syntax validation
		diags = syntaxCheck([]byte(content), filename)
	default:
		diags = s.diagnoseDefinitionFile(filename, content)
	}

	return hclDiagsToLSP(diags)
}

// diagnoseDefinitionFile parses a definition file (.kdef) with a best-effort eval context.
func (s *Server) diagnoseDefinitionFile(filename string, content string) hcl.Diagnostics {
	dir := filepath.Dir(filename)

	// Try to build an eval context from vars.kdef in the same directory
	ctx := s.buildEvalContext(dir)

	// Parse the file content using the eval context
	_, diags := parser.ParseBytes([]byte(content), filename, ctx)
	return diags
}

// buildEvalContext attempts to build an HCL eval context from vars.kdef.
// Returns a minimal context if vars.kdef doesn't exist or fails to parse.
func (s *Server) buildEvalContext(dir string) *hcl.EvalContext {
	vars := make(map[string]types.VariableDecl)

	// Try local vars.kdef
	varsFile := filepath.Join(dir, "vars.kdef")
	if _, err := os.Stat(varsFile); err == nil {
		// Check if we have an open (unsaved) version
		varsURI := pathToURI(varsFile)
		if doc := s.documents.Get(varsURI); doc != nil {
			parsed, diags := parser.ParseVariableFileFromBytes([]byte(doc.Content), varsFile)
			if !diags.HasErrors() {
				for k, v := range parsed {
					vars[k] = v
				}
			}
		} else {
			parsed, diags := parser.ParseVariableFile(varsFile)
			if !diags.HasErrors() {
				for k, v := range parsed {
					vars[k] = v
				}
			}
		}
	}

	// Try root-level vars.kdef (one directory up, if root.kdef exists there)
	parentDir := filepath.Dir(dir)
	rootFile := filepath.Join(parentDir, "root.kdef")
	if _, err := os.Stat(rootFile); err == nil {
		rootVarsFile := filepath.Join(parentDir, "vars.kdef")
		if _, err := os.Stat(rootVarsFile); err == nil {
			parsed, diags := parser.ParseVariableFile(rootVarsFile)
			if !diags.HasErrors() {
				for k, v := range parsed {
					if _, exists := vars[k]; !exists {
						vars[k] = v
					}
				}
			}
		}
	}

	// Scan for images — root-level first, then local overrides
	images := make(map[string]string)
	if _, err := os.Stat(filepath.Join(parentDir, "root.kdef")); err == nil {
		rootImages := scanImagesSafe(parentDir)
		for k, v := range rootImages {
			images[k] = v
		}
	}
	localImages := scanImagesSafe(dir)
	for k, v := range localImages {
		images[k] = v
	}

	// Build context — use lenient mode (empty overrides, no extra values)
	ctx, _ := parser.BuildEvalContext(vars, nil, nil, images, dir)
	if ctx == nil {
		// Fallback: minimal context with just env and functions
		ctx, _ = parser.BuildEvalContext(make(map[string]types.VariableDecl), nil, nil, nil, dir)
	}
	return ctx
}

// syntaxCheck does a basic HCL syntax check without schema validation.
func syntaxCheck(src []byte, filename string) hcl.Diagnostics {
	p := hclparse.NewParser()
	_, diags := p.ParseHCL(src, filename)
	return diags
}

// hclDiagsToLSP converts HCL diagnostics to LSP diagnostics.
func hclDiagsToLSP(diags hcl.Diagnostics) []protocol.Diagnostic {
	result := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		lspDiag := protocol.Diagnostic{
			Message: d.Summary,
			Source:  ptrStr("kdef"),
		}

		if d.Detail != "" {
			lspDiag.Message = d.Summary + ": " + d.Detail
		}

		switch d.Severity {
		case hcl.DiagError:
			lspDiag.Severity = ptrSeverity(protocol.DiagnosticSeverityError)
		case hcl.DiagWarning:
			lspDiag.Severity = ptrSeverity(protocol.DiagnosticSeverityWarning)
		default:
			lspDiag.Severity = ptrSeverity(protocol.DiagnosticSeverityInformation)
		}

		if d.Subject != nil {
			lspDiag.Range = hclRangeToLSP(*d.Subject)
		}

		result = append(result, lspDiag)
	}
	return result
}

// hclRangeToLSP converts an HCL source range to an LSP range.
// HCL uses 1-based lines/columns, LSP uses 0-based.
func hclRangeToLSP(r hcl.Range) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(max(0, r.Start.Line-1)),
			Character: uint32(max(0, r.Start.Column-1)),
		},
		End: protocol.Position{
			Line:      uint32(max(0, r.End.Line-1)),
			Character: uint32(max(0, r.End.Column-1)),
		},
	}
}

// uriToPath converts a file:// URI to a filesystem path.
func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		u, err := url.Parse(uri)
		if err == nil {
			return u.Path
		}
	}
	return uri
}

// pathToURI converts a filesystem path to a file:// URI.
func pathToURI(path string) string {
	return "file://" + path
}

func ptrStr(s string) *string {
	return &s
}

func ptrSeverity(s protocol.DiagnosticSeverity) *protocol.DiagnosticSeverity {
	return &s
}

// scanImagesSafe scans a directory for images blocks, tolerating parse errors in individual files.
func scanImagesSafe(dir string) map[string]string {
	images := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return images
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdef") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		found := parser.ScanImagesFromBytes(data, entry.Name())
		for k, v := range found {
			images[k] = v
		}
	}
	return images
}
