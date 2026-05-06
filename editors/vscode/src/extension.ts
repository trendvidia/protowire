// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

import * as vscode from "vscode";
import { parse, PxfError } from "@trendvidia/protowire/pxf";

const LANGUAGE_ID = "pxf";

let diagnostics: vscode.DiagnosticCollection;

export function activate(context: vscode.ExtensionContext): void {
  diagnostics = vscode.languages.createDiagnosticCollection(LANGUAGE_ID);
  context.subscriptions.push(diagnostics);

  // Validate already-open .pxf documents on activation.
  for (const doc of vscode.workspace.textDocuments) {
    validate(doc);
  }

  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument(validate),
    vscode.workspace.onDidChangeTextDocument((e) => validate(e.document)),
    vscode.workspace.onDidCloseTextDocument((doc) => diagnostics.delete(doc.uri)),
  );
}

export function deactivate(): void {
  diagnostics?.dispose();
}

function validate(doc: vscode.TextDocument): void {
  if (doc.languageId !== LANGUAGE_ID) {
    return;
  }
  const issues: vscode.Diagnostic[] = [];
  try {
    parse(doc.getText());
  } catch (err) {
    issues.push(toDiagnostic(doc, err));
  }
  diagnostics.set(doc.uri, issues);
}

function toDiagnostic(doc: vscode.TextDocument, err: unknown): vscode.Diagnostic {
  if (err instanceof PxfError) {
    // PxfError formats `<line>:<col>: <message>`. Strip the redundant prefix —
    // VS Code already shows the position next to the squiggle.
    const prefix = `${err.pos.line}:${err.pos.column}: `;
    const message = err.message.startsWith(prefix)
      ? err.message.slice(prefix.length)
      : err.message;
    return new vscode.Diagnostic(rangeAt(doc, err.pos.line, err.pos.column), message, vscode.DiagnosticSeverity.Error);
  }
  // Defensive: never let an unexpected parser failure bubble up to VS Code.
  const message = err instanceof Error ? err.message : String(err);
  return new vscode.Diagnostic(rangeAt(doc, 1, 1), `PXF parse error: ${message}`, vscode.DiagnosticSeverity.Error);
}

function rangeAt(doc: vscode.TextDocument, line: number, column: number): vscode.Range {
  // Parser positions are 1-based; vscode.Position is 0-based.
  const lineIdx = Math.max(0, Math.min(doc.lineCount - 1, line - 1));
  const lineText = doc.lineAt(lineIdx).text;
  const colIdx = Math.max(0, Math.min(lineText.length, column - 1));
  const start = new vscode.Position(lineIdx, colIdx);
  const end = new vscode.Position(lineIdx, Math.min(lineText.length, colIdx + 1));
  return new vscode.Range(start, end);
}
