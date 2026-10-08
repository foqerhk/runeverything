'use strict';
// KoKo IDE Bridge: lets the RunEverything Agent open/focus a given Cursor agent chat
// so phone input can be delivered into it. Loopback only, token-protected.
const vscode = require('vscode');
const http = require('http');
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');

let server;
let regFile;

// Only agent-chat UI commands; never terminal, file or extension management commands.
const COMMAND_ALLOW = ['composerMode.', 'composer.createNew', 'composer.newAgentChat', 'composer.openModelToggle', 'composer.openModeMenu', 'composer.focusComposer'];

function plain(v) {
  if (v === undefined || v === null) return null;
  if (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') return v;
  try { return JSON.parse(JSON.stringify(v)); } catch (_) { return String(v); }
}

function activate(context) {
  const token = crypto.randomBytes(24).toString('hex');
  const dir = path.join(os.homedir(), '.runeverything', 'ide-bridge');

  server = http.createServer((req, res) => {
    const reply = (code, obj) => {
      res.writeHead(code, { 'content-type': 'application/json' });
      res.end(JSON.stringify(obj));
    };
    if (req.method !== 'POST' || req.headers['x-koko-token'] !== token) {
      return reply(403, { ok: false, error: 'forbidden' });
    }
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
      if (body.length > 65536) req.destroy();
    });
    req.on('end', async () => {
      let msg = {};
      try { msg = JSON.parse(body || '{}'); } catch (_) { /* empty */ }
      try {
        switch (req.url) {
          case '/ping':
            break;
          case '/focus':
            await vscode.commands.executeCommand('composer.focusComposer', msg.composerId);
            break;
          case '/cancel':
            await vscode.commands.executeCommand('composer.cancelChat', msg.composerId);
            break;
          case '/command': {
            const id = String(msg.id || '');
            if (!COMMAND_ALLOW.some((p) => id.startsWith(p))) {
              return reply(403, { ok: false, error: `command not allowed: ${id}` });
            }
            if (msg.composerId) {
              await vscode.commands.executeCommand('composer.focusComposer', msg.composerId);
            }
            const result = await vscode.commands.executeCommand(id, ...(Array.isArray(msg.args) ? msg.args : []));
            return reply(200, { ok: true, focused: vscode.window.state.focused, result: plain(result) });
          }
          default:
            return reply(404, { ok: false, error: 'not found' });
        }
        reply(200, { ok: true, focused: vscode.window.state.focused });
      } catch (e) {
        reply(500, { ok: false, error: String((e && e.message) || e) });
      }
    });
  });

  server.listen(0, '127.0.0.1', () => {
    fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
    regFile = path.join(dir, `${vscode.env.uriScheme}-${process.pid}.json`);
    const write = () => {
      const reg = {
        version: context.extension.packageJSON.version,
        app: vscode.env.appName,
        scheme: vscode.env.uriScheme,
        pid: process.pid,
        ppid: process.ppid,
        port: server.address().port,
        token,
        folders: (vscode.workspace.workspaceFolders || []).map((f) => f.uri.fsPath),
        focused: vscode.window.state.focused,
        updatedMs: Date.now(),
      };
      try { fs.writeFileSync(regFile, JSON.stringify(reg), { mode: 0o600 }); } catch (_) { /* best effort */ }
    };
    write();
    context.subscriptions.push(
      vscode.workspace.onDidChangeWorkspaceFolders(write),
      vscode.window.onDidChangeWindowState(write),
    );
  });
}

function deactivate() {
  try { if (server) server.close(); } catch (_) { /* ignore */ }
  try { if (regFile) fs.unlinkSync(regFile); } catch (_) { /* ignore */ }
}

module.exports = { activate, deactivate };
