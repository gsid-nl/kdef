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
	case basename == "vars.kdef" || isVariableFile([]byte(content), filename):
		_, diags = parser.ParseVariableFileFromBytes([]byte(content), filename)
	case basename == "root.kdef":
		// Root files have a different schema; just do syntax validation
		diags = syntaxCheck([]byte(content), filename)
	default:
		diags = s.diagnoseDefinitionFile(filename, content)
	}

	return hclDiagsToLSP(diags)
}

// isVariableFile reports whether a file should be checked against the variable
// schema rather than the definition schema. A file imported by vars.kdef can be
// named anything (vars/sites.kdef, defaults.kdef, images.kdef), so the name is
// not enough — sniff for a top-level `variable` block instead.
func isVariableFile(src []byte, filename string) bool {
	p := hclparse.NewParser()
	file, diags := p.ParseHCL(src, filename)
	if diags.HasErrors() || file == nil {
		return false
	}
	content, _, _ := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
		},
	})
	return content != nil && len(content.Blocks) > 0
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

// buildEvalContext attempts to build an HCL eval context from vars.kdef and
// images {} blocks visible to the given directory. It walks upward to find
// the project root (directory containing root.kdef), then merges vars/images
// from every level on the path, with deeper levels winning on name collision.
// Returns a minimal context on failure.
func (s *Server) buildEvalContext(dir string) *hcl.EvalContext {
	rootDir := findRootDir(dir)

	loaded, _ := parser.LoadVariablesWalk(dir, rootDir, nil)
	vars := loaded.Variables
	if vars == nil {
		vars = make(map[string]types.VariableDecl)
	}

	images, _ := parser.ScanImagesWalk(dir, rootDir)
	if images == nil {
		images = make(map[string]string)
	}

	// Build context — use lenient mode (empty overrides, no extra values)
	ctx, _ := parser.BuildEvalContext(vars, nil, nil, images, dir)
	if ctx == nil {
		// Fallback: minimal context with just env and functions
		ctx, _ = parser.BuildEvalContext(make(map[string]types.VariableDecl), nil, nil, nil, dir)
	}
	return ctx
}

// findRootDir walks up from dir looking for a directory that contains
// root.kdef. Returns the found directory, or "" if none is found before
// reaching the filesystem root.
func findRootDir(dir string) string {
	cur, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(cur, "root.kdef")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
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

