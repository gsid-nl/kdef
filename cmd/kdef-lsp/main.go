package main

import (
	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp/server"

	"github.com/gsid-nl/kdef/internal/lsp"
)

func main() {
	commonlog.Configure(1, nil)

	s := lsp.NewServer()
	srv := server.NewServer(s.Handler(), "kdef-lsp", false)
	srv.RunStdio()
}
