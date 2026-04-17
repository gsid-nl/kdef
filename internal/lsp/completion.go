package lsp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/gsid-nl/kdef/internal/parser"
	"github.com/gsid-nl/kdef/internal/types"
)

var blockTypePattern = regexp.MustCompile(`^\s*(deployment|cronjob|configmap|secret|sealedsecret|persistentvolumeclaim|images|container|init|sidecar|scale|service|ingress|autoscale|port|env|env_from|resources|volume|security_context)\b`)

func (s *Server) completion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	uri := string(params.TextDocument.URI)
	doc := s.documents.Get(uri)
	if doc == nil {
		return nil, nil
	}

	line := int(params.Position.Line)
	col := int(params.Position.Character)
	lines := strings.Split(doc.Content, "\n")
	if line >= len(lines) {
		return nil, nil
	}

	currentLine := lines[line]
	before := ""
	if col <= len(currentLine) {
		before = currentLine[:col]
	}

	// --- Context-specific completions based on what's before the cursor ---

	// var.<cursor> — suggest known variable names
	if strings.HasSuffix(before, "var.") || varDotPartial(before) {
		return s.completeVariables(uri), nil
	}

	// env.<cursor> — suggest known env variable names
	if strings.HasSuffix(before, "env.") || envDotPartial(before) {
		return completeEnvVars(), nil
	}

	// image("<cursor> or image(<cursor> — suggest image names with quotes
	if imageCallPartial(before) {
		return s.completeImages(uri), nil
	}

	// secret("<cursor> or secret(<cursor> — suggest secret names
	if secretCallPartial(before) {
		return s.completeSecretNames(uri), nil
	}

	// configmap("<cursor> — suggest configmap names
	if configmapCallPartial(before) {
		return s.completeConfigMapNames(uri), nil
	}

	// After = sign (with optional space), suggest value expressions
	if afterEquals(before) {
		return s.completeValues(uri), nil
	}

	// --- Structural completions based on block context ---

	blockCtx := findBlockContext(lines, line)

	// Check if we're on an attribute line that already has content
	trimmed := strings.TrimSpace(before)
	if strings.Contains(trimmed, "=") {
		// Already typing a value — suggest value expressions
		return s.completeValues(uri), nil
	}

	switch {
	case blockCtx == "":
		return completeTopLevelBlocks(), nil
	case blockCtx == "env":
		// Inside env block — free-form keys, no schema suggestions
		return nil, nil
	default:
		return completeBlockMembers(blockCtx), nil
	}
}

// findBlockContext scans backwards from the cursor to find the nearest enclosing block type.
func findBlockContext(lines []string, cursorLine int) string {
	depth := 0
	for i := cursorLine; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		for _, ch := range line {
			if ch == '}' {
				depth++
			}
		}
		for _, ch := range line {
			if ch == '{' {
				if depth > 0 {
					depth--
				} else {
					matches := blockTypePattern.FindStringSubmatch(lines[i])
					if matches != nil {
						return matches[1]
					}
					return ""
				}
			}
		}
	}
	return ""
}

func completeTopLevelBlocks() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, b := range topLevelBlocks {
		kind := protocol.CompletionItemKindClass
		snippet := b.Type
		if b.Labels > 0 {
			snippet = b.Type + ` "$1" {\n\t$0\n}`
		} else {
			snippet = b.Type + ` {\n\t$0\n}`
		}
		format := protocol.InsertTextFormatSnippet
		items = append(items, protocol.CompletionItem{
			Label:            b.Type,
			Kind:             &kind,
			Detail:           ptrStr(b.Doc),
			InsertText:       ptrStr(snippet),
			InsertTextFormat: &format,
		})
	}
	return items
}

func completeBlockMembers(blockType string) []protocol.CompletionItem {
	schema, ok := blockSchemaMap[blockType]
	if !ok {
		return nil
	}

	var items []protocol.CompletionItem

	for _, attr := range schema.Attributes {
		kind := protocol.CompletionItemKindProperty
		detail := attr.Doc
		if attr.Required {
			detail = "(required) " + detail
		}
		format := protocol.InsertTextFormatSnippet
		items = append(items, protocol.CompletionItem{
			Label:            attr.Name,
			Kind:             &kind,
			Detail:           ptrStr(detail),
			InsertText:       ptrStr(attr.Name + " = $0"),
			InsertTextFormat: &format,
		})
	}

	for _, sub := range schema.SubBlocks {
		kind := protocol.CompletionItemKindStruct
		format := protocol.InsertTextFormatSnippet
		snippet := sub.Type
		if sub.Labels > 0 {
			snippet = sub.Type + ` "$1" {\n\t$0\n}`
		} else {
			snippet = sub.Type + ` {\n\t$0\n}`
		}
		items = append(items, protocol.CompletionItem{
			Label:            sub.Type,
			Kind:             &kind,
			Detail:           ptrStr(sub.Doc),
			InsertText:       ptrStr(snippet),
			InsertTextFormat: &format,
		})
	}

	return items
}

// completeValues suggests value expressions: functions, var., env., and context-specific values.
func (s *Server) completeValues(uri string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// Variable namespaces
	kind := protocol.CompletionItemKindVariable
	items = append(items, protocol.CompletionItem{
		Label:  "var.",
		Kind:   &kind,
		Detail: ptrStr("Project variables (from vars.kdef)"),
	})
	items = append(items, protocol.CompletionItem{
		Label:  "env.",
		Kind:   &kind,
		Detail: ptrStr("Local environment variables"),
	})

	// Functions
	fnKind := protocol.CompletionItemKindFunction
	format := protocol.InsertTextFormatSnippet
	for _, fn := range builtinFunctions {
		items = append(items, protocol.CompletionItem{
			Label:            fn.Name + "()",
			Kind:             &fnKind,
			Detail:           ptrStr(fn.Doc),
			InsertText:       ptrStr(fn.Signature),
			InsertTextFormat: &format,
		})
	}

	// Add known namespace values
	for _, ns := range s.getNamespaces(uri) {
		nsKind := protocol.CompletionItemKindEnum
		items = append(items, protocol.CompletionItem{
			Label:      ns,
			Kind:       &nsKind,
			Detail:     ptrStr("namespace"),
			InsertText: ptrStr(`"` + ns + `"`),
		})
	}

	return items
}

func (s *Server) completeVariables(uri string) []protocol.CompletionItem {
	filename := uriToPath(uri)
	dir := filepath.Dir(filename)

	allVars := make(map[string]string) // name -> doc

	loadVarsFrom := func(varsFile string) {
		varsURI := pathToURI(varsFile)
		if doc := s.documents.Get(varsURI); doc != nil {
			parsed, diags := parser.ParseVariableFileFromBytes([]byte(doc.Content), varsFile)
			if !diags.HasErrors() {
				for name, v := range parsed {
					allVars[name] = varDoc(v)
				}
			}
		} else if _, err := os.Stat(varsFile); err == nil {
			parsed, diags := parser.ParseVariableFile(varsFile)
			if !diags.HasErrors() {
				for name, v := range parsed {
					allVars[name] = varDoc(v)
				}
			}
		}
	}

	// Root-level vars (lower precedence)
	parentDir := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parentDir, "root.kdef")); err == nil {
		loadVarsFrom(filepath.Join(parentDir, "vars.kdef"))
	}

	// Local vars (higher precedence, overwrites)
	loadVarsFrom(filepath.Join(dir, "vars.kdef"))

	var items []protocol.CompletionItem
	kind := protocol.CompletionItemKindVariable
	for name, doc := range allVars {
		items = append(items, protocol.CompletionItem{
			Label:  name,
			Kind:   &kind,
			Detail: ptrStr(doc),
		})
	}
	return items
}

func completeEnvVars() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	kind := protocol.CompletionItemKindConstant
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			detail := v
			if len(detail) > 50 {
				detail = detail[:50] + "..."
			}
			items = append(items, protocol.CompletionItem{
				Label:  k,
				Kind:   &kind,
				Detail: ptrStr(detail),
			})
		}
	}
	return items
}

func (s *Server) completeImages(uri string) []protocol.CompletionItem {
	images := s.scanAllImages(uri)

	var items []protocol.CompletionItem
	kind := protocol.CompletionItemKindValue
	for name, ref := range images {
		items = append(items, protocol.CompletionItem{
			Label:  name,
			Kind:   &kind,
			Detail: ptrStr(ref),
		})
	}
	return items
}

func (s *Server) completeSecretNames(uri string) []protocol.CompletionItem {
	names := s.scanSecretNames(uri)

	var items []protocol.CompletionItem
	kind := protocol.CompletionItemKindValue
	for _, name := range names {
		items = append(items, protocol.CompletionItem{
			Label:  name,
			Kind:   &kind,
			Detail: ptrStr("secret"),
		})
	}
	return items
}

func (s *Server) completeConfigMapNames(uri string) []protocol.CompletionItem {
	names := s.scanConfigMapNames(uri)

	var items []protocol.CompletionItem
	kind := protocol.CompletionItemKindValue
	for _, name := range names {
		items = append(items, protocol.CompletionItem{
			Label:  name,
			Kind:   &kind,
			Detail: ptrStr("configmap"),
		})
	}
	return items
}

// --- Data scanning helpers ---

func (s *Server) scanAllImages(uri string) map[string]string {
	filename := uriToPath(uri)
	dir := filepath.Dir(filename)
	images := make(map[string]string)

	scanDir := func(d string) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdef") {
				continue
			}
			data := readFileBytes(filepath.Join(d, entry.Name()))
			for k, v := range parser.ScanImagesFromBytes(data, entry.Name()) {
				images[k] = v
			}
		}
	}

	// Root first (if multi-project)
	parentDir := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parentDir, "root.kdef")); err == nil {
		scanDir(parentDir)
	}
	scanDir(dir)

	// Also scan open (unsaved) documents
	for _, doc := range s.documents.GetAllInDir(pathToURI(dir)) {
		for k, v := range parser.ScanImagesFromBytes([]byte(doc.Content), uriToPath(doc.URI)) {
			images[k] = v
		}
	}

	return images
}

// scanSecretNames finds all secret and sealedsecret block names in the project.
func (s *Server) scanSecretNames(uri string) []string {
	return s.scanBlockNames(uri, "secret", "sealedsecret")
}

// scanConfigMapNames finds all configmap block names in the project.
func (s *Server) scanConfigMapNames(uri string) []string {
	return s.scanBlockNames(uri, "configmap")
}

var blockNamePattern = regexp.MustCompile(`^\s*(?:secret|sealedsecret|configmap)\s+"([^"]+)"`)

func (s *Server) scanBlockNames(uri string, types ...string) []string {
	filename := uriToPath(uri)
	dir := filepath.Dir(filename)
	nameSet := make(map[string]bool)

	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	pattern := regexp.MustCompile(`^\s*(` + strings.Join(types, "|") + `)\s+"([^"]+)"`)

	scanContent := func(content string) {
		for _, line := range strings.Split(content, "\n") {
			if m := pattern.FindStringSubmatch(line); m != nil {
				nameSet[m[2]] = true
			}
		}
	}

	// Scan files from disk in dir and parent (for root project)
	scanDirFiles := func(d string) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdef") {
				continue
			}
			data := readFileBytes(filepath.Join(d, entry.Name()))
			if data != nil {
				scanContent(string(data))
			}
		}
	}

	parentDir := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parentDir, "root.kdef")); err == nil {
		scanDirFiles(parentDir)
	}
	scanDirFiles(dir)

	// Scan open documents
	for _, doc := range s.documents.GetAllInDir(pathToURI(dir)) {
		scanContent(doc.Content)
	}

	var names []string
	for name := range nameSet {
		names = append(names, name)
	}
	return names
}

// getNamespaces reads namespace values from root.kdef.
func (s *Server) getNamespaces(uri string) []string {
	filename := uriToPath(uri)
	dir := filepath.Dir(filename)

	// Check for root.kdef in current dir or parent
	for _, d := range []string{dir, filepath.Dir(dir)} {
		rootFile := filepath.Join(d, "root.kdef")
		data := readFileBytes(rootFile)
		if data == nil {
			continue
		}
		// Simple regex extraction for namespaces = ["a", "b"]
		content := string(data)
		re := regexp.MustCompile(`namespaces\s*=\s*\[([^\]]+)\]`)
		m := re.FindStringSubmatch(content)
		if m == nil {
			continue
		}
		var namespaces []string
		for _, part := range strings.Split(m[1], ",") {
			ns := strings.TrimSpace(part)
			ns = strings.Trim(ns, `"'`)
			if ns != "" {
				namespaces = append(namespaces, ns)
			}
		}
		return namespaces
	}
	return nil
}

// --- Cursor context helpers ---

func varDotPartial(s string) bool {
	i := strings.LastIndex(s, "var.")
	if i < 0 {
		return false
	}
	after := s[i+4:]
	for _, ch := range after {
		if !isWordChar(ch) {
			return false
		}
	}
	return true
}

func envDotPartial(s string) bool {
	i := strings.LastIndex(s, "env.")
	if i < 0 {
		return false
	}
	after := s[i+4:]
	for _, ch := range after {
		if !isWordChar(ch) {
			return false
		}
	}
	return true
}

func imageCallPartial(s string) bool {
	for _, prefix := range []string{`image("`, `image(`} {
		i := strings.LastIndex(s, prefix)
		if i >= 0 {
			after := s[i+len(prefix):]
			return !strings.Contains(after, ")")
		}
	}
	return false
}

func secretCallPartial(s string) bool {
	for _, prefix := range []string{`secret("`, `secret(`} {
		i := strings.LastIndex(s, prefix)
		if i >= 0 {
			after := s[i+len(prefix):]
			// Only first arg (no comma yet)
			return !strings.Contains(after, ")") && !strings.Contains(after, ",")
		}
	}
	return false
}

func configmapCallPartial(s string) bool {
	for _, prefix := range []string{`configmap("`, `configmap(`} {
		i := strings.LastIndex(s, prefix)
		if i >= 0 {
			after := s[i+len(prefix):]
			return !strings.Contains(after, ")") && !strings.Contains(after, ",")
		}
	}
	return false
}

func afterEquals(s string) bool {
	trimmed := strings.TrimRight(s, " \t")
	return strings.HasSuffix(trimmed, "=")
}

func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func readFileBytes(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func varDoc(v types.VariableDecl) string {
	doc := "type: " + v.Type
	if v.Default != nil {
		doc += " (has default)"
	} else {
		doc += " (required)"
	}
	return doc
}
