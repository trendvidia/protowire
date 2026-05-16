// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

const LANGUAGE_ID = "pxf";

// REGISTRY_URI_SCHEME is the URI scheme protolsp uses for virtual
// documents backed by registry-fetched .proto sources. When
// textDocument/definition resolves to a file that isn't on disk under
// the workspace root, protolsp returns a URI like
//   registry://<namespace>/<file-path>
// and we render it via the TextDocumentContentProvider registered
// below — it sends the URI back to the LSP as a protolsp/sourceContent
// request and the server replies with bytes fetched through
// client.Resolver.GetSource.
const REGISTRY_URI_SCHEME = "registry";

// SOURCE_CONTENT_METHOD is the custom LSP method protolsp implements
// to fetch bytes for a registry:// URI. Defined in
// protolsp/internal/server/source_content.go.
const SOURCE_CONTENT_METHOD = "protolsp/sourceContent";

interface SourceContentResponse {
  content: string;
}

let client: LanguageClient | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const cfg = vscode.workspace.getConfiguration("protolsp");
  const serverCommand = resolveServerCommand(cfg, context);

  const serverOptions: ServerOptions = {
    command: serverCommand.command,
    args: serverCommand.args,
    options: {
      env: process.env,
    },
  };

  // initializationOptions are forwarded to the LSP server's
  // dispatchInitialize → schema.Open path. Empty values mean
  // "no registry" — protolsp degrades to parse-only diagnostics.
  const initializationOptions = {
    registry: {
      address: cfg.get<string>("registry.address", ""),
      namespace: cfg.get<string>("registry.namespace", ""),
      token: cfg.get<string>("registry.token", ""),
    },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: LANGUAGE_ID }],
    initializationOptions,
    // synchronize: stay simple — the server tracks its own document state
    // via didOpen/didChange/didClose, so we don't need filesystem watches
    // or other sync features in M6.
  };

  client = new LanguageClient(
    "protolsp",
    "PXF Language Server",
    serverOptions,
    clientOptions,
  );

  // Surface server crashes / spawn failures as a single notification with
  // a "Show output" affordance instead of letting them fail silently.
  // VS Code will retry per its built-in restart policy.
  try {
    await client.start();
  } catch (err) {
    const message =
      err instanceof Error ? err.message : String(err);
    vscode.window
      .showErrorMessage(
        `PXF: failed to start language server (${message}). Set 'protolsp.path' to your protolsp binary or install it on PATH.`,
        "Open Settings",
      )
      .then((choice) => {
        if (choice === "Open Settings") {
          void vscode.commands.executeCommand(
            "workbench.action.openSettings",
            "protolsp",
          );
        }
      });
    return;
  }

  // Register a content provider for registry:// URIs so go-to-definition
  // can open registry-backed .proto sources that don't exist on disk.
  // VS Code calls provideTextDocumentContent on every fresh open; we
  // round-trip the URI through the LSP's protolsp/sourceContent custom
  // request and return the bytes the server fetched via GetSource.
  //
  // Errors propagate as thrown exceptions — VS Code surfaces them in
  // the placeholder document and as a notification, which is what we
  // want for "registry unreachable" or "file no longer exists" cases.
  const registryContentProvider: vscode.TextDocumentContentProvider = {
    async provideTextDocumentContent(
      uri: vscode.Uri,
      token: vscode.CancellationToken,
    ): Promise<string> {
      if (!client) {
        throw new Error("PXF language server is not running");
      }
      const response = await client.sendRequest<SourceContentResponse>(
        SOURCE_CONTENT_METHOD,
        { uri: uri.toString() },
        token,
      );
      return response.content;
    },
  };
  context.subscriptions.push(
    vscode.workspace.registerTextDocumentContentProvider(
      REGISTRY_URI_SCHEME,
      registryContentProvider,
    ),
  );

  context.subscriptions.push({
    dispose: () => {
      void client?.stop();
    },
  });
}

export function deactivate(): Thenable<void> | undefined {
  return client?.stop();
}

interface ResolvedCommand {
  command: string;
  args: string[];
}

// resolveServerCommand picks the protolsp binary to spawn. Priority:
//   1. workspace setting `protolsp.path` (absolute or relative to the
//      workspace folder)
//   2. environment variable PROTOLSP_PATH
//   3. just "protolsp" — relies on $PATH lookup at spawn time. Users
//      who installed via `go install` typically have ~/go/bin on PATH.
//
// `context` is reserved for a future bundled-binary fallback (per-
// platform .vsix distributions that ship the binary inside the
// extension); for M6 the user installs the binary themselves.
function resolveServerCommand(
  cfg: vscode.WorkspaceConfiguration,
  _context: vscode.ExtensionContext,
): ResolvedCommand {
  const configured = cfg.get<string>("path", "").trim();
  const envPath = (process.env.PROTOLSP_PATH ?? "").trim();
  const command = configured || envPath || "protolsp";

  const argList: string[] = [];
  const logLevel = cfg.get<string>("logLevel", "").trim();
  if (logLevel) {
    argList.push(`--log-level=${logLevel}`);
  }

  return { command, args: argList };
}
