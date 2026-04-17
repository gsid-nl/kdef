package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/gsid-nl/kdef/internal/version"
)

const serverName = "kdef-lsp"

// Server is the kdef language server.
type Server struct {
	handler   protocol.Handler
	documents *DocumentStore
}

// NewServer creates a new kdef LSP server and returns the configured handler.
func NewServer() *Server {
	s := &Server{
		documents: NewDocumentStore(),
	}

	s.handler = protocol.Handler{
		Initialize:             s.initialize,
		Initialized:            s.initialized,
		Shutdown:               s.shutdown,
		SetTrace:               s.setTrace,
		TextDocumentDidOpen:    s.didOpen,
		TextDocumentDidChange:  s.didChange,
		TextDocumentDidClose:   s.didClose,
		TextDocumentDidSave:    s.didSave,
		TextDocumentCompletion: s.completion,
	}

	return s
}

// Handler returns the protocol handler for use with glsp server.
func (s *Server) Handler() *protocol.Handler {
	return &s.handler
}

func (s *Server) initialize(_ *glsp.Context, params *protocol.InitializeParams) (any, error) {
	syncKind := protocol.TextDocumentSyncKindFull

	capabilities := s.handler.CreateServerCapabilities()
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: ptrBool(true),
		Change:    &syncKind,
		Save: &protocol.SaveOptions{
			IncludeText: ptrBool(true),
		},
	}
	capabilities.CompletionProvider = &protocol.CompletionOptions{
		TriggerCharacters: []string{".", "=", `"`, "("},
	}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    serverName,
			Version: &version.Version,
		},
	}, nil
}

func (s *Server) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	return nil
}

func (s *Server) shutdown(_ *glsp.Context) error {
	return nil
}

func (s *Server) setTrace(_ *glsp.Context, _ *protocol.SetTraceParams) error {
	return nil
}

func (s *Server) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	content := params.TextDocument.Text
	s.documents.Open(uri, content, params.TextDocument.Version)
	s.publishDiagnostics(ctx, uri, content)
	return nil
}

func (s *Server) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	// We use full sync, so the last change contains the full content
	if len(params.ContentChanges) > 0 {
		change := params.ContentChanges[len(params.ContentChanges)-1]
		if content, ok := change.(protocol.TextDocumentContentChangeEventWhole); ok {
			s.documents.Update(uri, content.Text, params.TextDocument.Version)
			s.publishDiagnostics(ctx, uri, content.Text)
		}
	}
	return nil
}

func (s *Server) didSave(ctx *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	if params.Text != nil {
		s.documents.Update(uri, *params.Text, 0)
		s.publishDiagnostics(ctx, uri, *params.Text)
	}
	return nil
}

func (s *Server) didClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	s.documents.Close(uri)
	return nil
}

func (s *Server) publishDiagnostics(ctx *glsp.Context, uri string, content string) {
	diagnostics := s.diagnoseFile(uri, content)
	ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentUri(uri),
		Diagnostics: diagnostics,
	})
}

func ptrBool(b bool) *bool {
	return &b
}
