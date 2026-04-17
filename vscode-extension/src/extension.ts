import * as path from "path";
import {
  workspace,
  ExtensionContext,
} from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient;

export function activate(context: ExtensionContext) {
  const config = workspace.getConfiguration("kdef");
  const serverPath = config.get<string>("lsp.path", "kdef-lsp");

  const serverOptions: ServerOptions = {
    command: serverPath,
    args: [],
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "kdef" }],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/*.kdef"),
    },
  };

  client = new LanguageClient(
    "kdef",
    "kdef Language Server",
    serverOptions,
    clientOptions
  );

  client.start();
}

export function deactivate(): Thenable<void> | undefined {
  if (!client) {
    return undefined;
  }
  return client.stop();
}
