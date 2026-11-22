'use strict';
// Castmill OS kiosk shell (Electron main process).
//
// One process per display output (castmill-player@<output>.service). The
// compositor pins the window to its output by Wayland app ID
// "castmill-output-<output>" (ADR-0004). The process:
//   - shows the local status page (pairing code, errors) until the device is
//     enrolled, then the Castmill player URL from the agent;
//   - sends a "frame rendered" heartbeat to castmill-agent, plus GPU/decode
//     capabilities and playback stats;
//   - answers screenshot and reload requests from the agent;
//   - recovers from renderer crashes and hangs (spec 05, layer 1).
// Security (spec 04): contextIsolation + sandbox, no Node.js in renderers,
// navigation limited to the player origin, permissions denied by default.

const { app, BrowserWindow, ipcMain, session, powerSaveBlocker } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
const { AgentLink } = require('./agentlink');
const { decideView, isNavigationAllowed, outputFromArgv } = require('./policy');

const OUTPUT = outputFromArgv(process.argv);
const APP_ID = `castmill-output-${OUTPUT}`;
const SOCKET = process.env.CASTMILL_AGENT_SOCKET || '/run/castmill-agent/player.sock';
const PLAYER_CONFIG = process.env.CASTMILL_PLAYER_CONFIG || '/data/castmill/config/agent/player.json';
const STATUS_PAGE = path.join(__dirname, 'status.html');
const RETRY_LOAD_MS = 30_000;
// Development only: a normal window instead of a fullscreen kiosk window,
// for running the shell on a workstation (see player/README.md).
const WINDOWED = process.env.CASTMILL_WINDOWED === '1';
const HANG_RELOAD_MS = 10_000;

// Wayland app ID: Chromium derives it from the desktop file name.
app.setName(APP_ID);
if (process.platform === 'linux') app.setDesktopName(`${APP_ID}.desktop`);
if (process.env.CASTMILL_USER_DATA) app.setPath('userData', path.join(process.env.CASTMILL_USER_DATA, OUTPUT));

// No Electron auto-updater: the player updates with the OS image (ADR-0003).
app.commandLine.appendSwitch('autoplay-policy', 'no-user-gesture-required');
app.commandLine.appendSwitch('disable-features', 'Translate,MediaRouter');

function log(...a) {
  console.log(`[${OUTPUT}]`, ...a);
}

function readPlayerConfig() {
  try {
    return JSON.parse(fs.readFileSync(PLAYER_CONFIG, 'utf8'));
  } catch {
    return {};
  }
}

let win = null;
let agentState = null;
let current = { view: null, url: null };
let hangTimer = null;
let retryTimer = null;
let devBadge = false;

const agent = new AgentLink(SOCKET, OUTPUT);

function createWindow() {
  win = new BrowserWindow({
    title: APP_ID,
    show: true,
    fullscreen: !WINDOWED,
    frame: WINDOWED,
    kiosk: !WINDOWED,
    width: 960,
    height: 540,
    backgroundColor: '#000000',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      webviewTag: false,
      spellcheck: false,
      backgroundThrottling: false,
    },
  });
  win.removeMenu();
  const wc = win.webContents;

  wc.setWindowOpenHandler(() => ({ action: 'deny' }));
  wc.on('will-navigate', (e, url) => {
    if (!isNavigationAllowed(url, current.url, readPlayerConfig().allowed_origins || [])) {
      log('blocked navigation to', url);
      e.preventDefault();
    }
  });
  wc.on('will-redirect', (e, url) => {
    if (!isNavigationAllowed(url, current.url, readPlayerConfig().allowed_origins || [])) {
      log('blocked redirect to', url);
      e.preventDefault();
    }
  });
  wc.on('render-process-gone', (_e, details) => {
    log('renderer gone:', details.reason);
    setTimeout(() => reloadCurrent(), 1000);
  });
  win.on('unresponsive', () => {
    log('window unresponsive');
    clearTimeout(hangTimer);
    hangTimer = setTimeout(() => {
      log('still unresponsive; reloading');
      wc.forcefullyCrashRenderer();
      reloadCurrent();
    }, HANG_RELOAD_MS);
  });
  win.on('responsive', () => clearTimeout(hangTimer));
  wc.on('did-fail-load', (_e, code, desc, url, isMainFrame) => {
    if (!isMainFrame || code === -3 /* aborted */) return;
    log(`load failed (${code} ${desc}): ${url}`);
    showStatus({ reason: 'offline', detail: desc });
    clearTimeout(retryTimer);
    retryTimer = setTimeout(() => {
      current = { view: null, url: null };
      apply();
    }, RETRY_LOAD_MS);
  });
  wc.on('did-finish-load', () => {
    if (devBadge && current.view === 'content') {
      wc.insertCSS(
        'html::after{content:"DEVELOPMENT IMAGE";position:fixed;right:8px;bottom:8px;z-index:2147483647;' +
          'background:#c00;color:#fff;font:bold 14px sans-serif;padding:4px 8px;pointer-events:none}',
      ).catch(() => {});
    }
    if (current.view === 'status') wc.send('castmill:state', statusPayload());
  });
  // Hide the cursor: it's a signage screen.
  wc.on('dom-ready', () => wc.insertCSS('*{cursor:none !important}').catch(() => {}));
}

let lastStatus = { reason: 'starting' };

function statusPayload() {
  return { ...lastStatus, output: OUTPUT, agent: agentState, agentConnected: agent.connected };
}

function showStatus(s) {
  lastStatus = s;
  if (current.view !== 'status') {
    current = { view: 'status', url: null };
    win.loadFile(STATUS_PAGE).catch((e) => log('status page:', e.message));
  } else {
    win.webContents.send('castmill:state', statusPayload());
  }
}

function apply() {
  if (!win) return;
  const d = decideView(agentState);
  devBadge = Boolean(agentState?.dev_image);
  if (d.view === 'status') {
    showStatus({ reason: d.reason });
    return;
  }
  if (current.view === 'content' && current.url === d.url) return;
  clearTimeout(retryTimer);
  current = { view: 'content', url: d.url };
  log('loading player', d.url);
  win.loadURL(d.url).catch(() => {}); // failures handled in did-fail-load
}

function reloadCurrent() {
  if (!win || win.isDestroyed()) return;
  if (current.view === 'content') {
    current = { view: null, url: null };
    apply();
  } else {
    win.webContents.reload();
  }
}

agent.on('message', async (m) => {
  switch (m.type) {
    case 'state':
      agentState = m.state;
      apply();
      break;
    case 'reload':
      log('reload requested by agent');
      reloadCurrent();
      break;
    case 'screenshot':
      try {
        const img = await win.webContents.capturePage();
        agent.send({ type: 'screenshot', id: m.id, png: img.toPNG().toString('base64') });
      } catch (e) {
        log('screenshot failed:', e.message);
        agent.send({ type: 'screenshot', id: m.id, png: '' });
      }
      break;
  }
});
agent.on('connect', () => {
  log('connected to agent');
  sendCapabilities();
});
agent.on('disconnect', () => {
  log('agent connection lost');
  if (current.view === 'status') showStatus(lastStatus);
});

// Frame heartbeat: requestAnimationFrame callbacks in the renderer prove that
// frames are being produced; forwarded to the agent (health check, watchdog).
ipcMain.on('castmill:frame', (e) => {
  if (win && e.sender === win.webContents) agent.send({ type: 'frame', output: OUTPUT });
});
ipcMain.on('castmill:stats', (e, stats) => {
  if (win && e.sender === win.webContents && stats && typeof stats === 'object') {
    agent.send({ type: 'stats', data: { url: current.url, ...stats } });
  }
});

async function sendCapabilities() {
  try {
    const info = await app.getGPUInfo('complete');
    agent.send({
      type: 'capabilities',
      data: {
        electron: process.versions.electron,
        chrome: process.versions.chrome,
        gpu_feature_status: app.getGPUFeatureStatus(),
        video_decoding: info.videoDecoding ?? info.auxAttributes?.videoDecodeAcceleratorSupportedProfile ?? null,
        gpu_devices: (info.gpuDevice || []).map((d) => ({ vendor: d.vendorId, device: d.deviceId, driver: d.driverVendor })),
        aux: info.auxAttributes ? { gl_renderer: info.auxAttributes.glRenderer, gl_version: info.auxAttributes.glVersion } : null,
      },
    });
  } catch (e) {
    log('GPU info:', e.message);
  }
}

app.whenReady().then(() => {
  powerSaveBlocker.start('prevent-display-sleep');
  // Deny all permission requests except what signage content needs.
  const allowed = new Set(['fullscreen', 'clipboard-sanitized-write']);
  session.defaultSession.setPermissionRequestHandler((_wc, perm, cb) => cb(allowed.has(perm)));
  session.defaultSession.setPermissionCheckHandler((_wc, perm) => allowed.has(perm));
  if (process.env.CASTMILL_PROXY_CA) log('note: custom proxy CA is not yet imported into Chromium (TODO)');
  createWindow();
  showStatus({ reason: 'starting' });
  agent.start();
});

// The process lives as long as its window; systemd restarts it.
app.on('window-all-closed', () => app.quit());
process.on('SIGTERM', () => {
  agent.close();
  app.quit();
});
