'use strict';
// Castmill OS kiosk shell (Electron main process).
//
// One process per display output (castmill-player@<output>.service). The
// compositor pins the window to its output by Wayland app ID
// "castmill-output-<output>" (ADR-0004). The process:
//   - loads the Castmill device app from the backend (player URL from
//     castmill-agent); the device app does pairing, content and commands;
//   - answers the device app's native calls through the castmillOS bridge
//     (Castmill's Machine interface, docs/castmill-integration.md);
//   - shows a local status page while the device app can't be loaded;
//   - sends a "frame rendered" heartbeat to castmill-agent;
//   - recovers from renderer crashes and hangs (spec 05, layer 1).
// Security (spec 04): contextIsolation + sandbox, no Node.js in renderers,
// navigation limited to the player origin, the bridge answers only that
// origin's main frame, permissions denied by default.

const { app, BrowserWindow, ipcMain, screen, session, powerSaveBlocker } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
const { AgentLink } = require('./agentlink');
const { MachineBridge } = require('./bridge');
const { probeGPU } = require('./gpu');
const { decideView, isNavigationAllowed, isTrustedBridgeSender, outputFromArgv } = require('./policy');

const OUTPUT = outputFromArgv(process.argv);
const APP_ID = `castmill-output-${OUTPUT}`;
const SOCKET = process.env.CASTMILL_AGENT_SOCKET || '/run/castmill-agent/player.sock';
const PLAYER_CONFIG = process.env.CASTMILL_PLAYER_CONFIG || '/data/castmill/config/agent/player.json';
const STATUS_PAGE = path.join(__dirname, 'status.html');
const RETRY_LOAD_MS = 30_000;
const HANG_RELOAD_MS = 10_000;
// Development only: a normal window instead of a fullscreen kiosk window,
// for running the shell on a workstation (see player/README.md).
const WINDOWED = process.env.CASTMILL_WINDOWED === '1';

// Wayland app ID: Chromium derives it from the desktop file name.
app.setName(APP_ID);
if (process.platform === 'linux') app.setDesktopName(`${APP_ID}.desktop`);
if (process.env.CASTMILL_USER_DATA) app.setPath('userData', path.join(process.env.CASTMILL_USER_DATA, OUTPUT));

// No Electron auto-updater: the shell updates with the OS image (ADR-0003).
app.commandLine.appendSwitch('autoplay-policy', 'no-user-gesture-required');
app.commandLine.appendSwitch('disable-features', 'Translate,MediaRouter');
// HTML-in-canvas (experimental): widgets can draw live HTML elements into a
// canvas, e.g. for WebGL effects on HTML content.
app.commandLine.appendSwitch('enable-blink-features', 'CanvasDrawElement');

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

const agent = new AgentLink(SOCKET, OUTPUT);
let bridge = null;

const devImage = () => Boolean(agentState?.dev_image);
const navigationAllowed = (url) =>
  isNavigationAllowed(url, current.url, readPlayerConfig().allowed_origins || [], devImage());

function restartPlayer() {
  log('restart requested by the Castmill device app');
  if (process.env.INVOCATION_ID) {
    app.exit(0); // systemd starts a fresh instance (Restart=always)
  } else {
    app.relaunch();
    app.exit(0);
  }
}

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
    if (!navigationAllowed(url)) {
      log('blocked navigation to', url);
      e.preventDefault();
    }
  });
  wc.on('will-redirect', (e, url) => {
    if (!navigationAllowed(url)) {
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
    // Not from inside the failing navigation's event: a load started here
    // can be dropped, leaving an unpainted window.
    setImmediate(() => showStatus({ reason: 'offline', detail: desc }));
    clearTimeout(retryTimer);
    retryTimer = setTimeout(() => {
      current = { view: null, url: null };
      apply();
    }, RETRY_LOAD_MS);
  });
  wc.on('did-finish-load', () => {
    log('showing', current.view === 'content' ? current.url : 'status page');
    if (devImage() && current.view === 'content') {
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
  if (d.view === 'status') {
    showStatus({ reason: d.reason });
    return;
  }
  if (current.view === 'content' && current.url === d.url) return;
  clearTimeout(retryTimer);
  current = { view: 'content', url: d.url };
  log('loading', d.url);
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

agent.on('message', (m) => {
  switch (m.type) {
    case 'state':
      agentState = m.state;
      apply();
      break;
    case 'reload':
      log('reload requested by agent');
      reloadCurrent();
      break;
  }
});
agent.on('connect', () => {
  log('connected to agent');
  // The agent keeps the schedule across reboots, but may have missed changes.
  const timers = bridge?.readSettings().TIMERS;
  if (timers !== undefined) sendTimers(timers);
  if (gpuReport) sendGPUReport();
});

// GPU report: hardware or software rendering, and the video formats decoded
// in hardware. Probed once the GPU process has started; sent to the agent and
// shown in Castmill's device details (getHardwareInfo).
const GPU_PROBE_DELAY_MS = 5_000;
const GPU_REPORT_WAIT_MS = 20_000;
let gpuReport = null;
let gpuReady = null; // resolves with the report (or null if the probe failed)
function sendGPUReport() {
  agent.call('gpu_report', gpuReport).catch((err) => log('GPU report not sent to the agent:', err.message));
}
function reportGPU() {
  gpuReady = new Promise((resolve) => {
    setTimeout(() => {
      probeGPU({ app, BrowserWindow })
        .then((r) => {
          gpuReport = r;
          log(`GPU: ${r.rendering} rendering (${r.renderer || 'unknown renderer'}),`,
            `hardware video decoding: ${r.video_decode.join(', ') || 'none'}`);
          if (agent.connected) sendGPUReport();
          resolve(r);
        })
        .catch((err) => {
          log('GPU probe failed:', err.message);
          resolve(null);
        });
    }, GPU_PROBE_DELAY_MS);
  });
}

// GPU and display facts for getHardwareInfo, e.g. display
// "HDMI-A-1, 3840 × 2160, 60 Hz".
async function shellHardware() {
  const r = await Promise.race([gpuReady, new Promise((res) => setTimeout(res, GPU_REPORT_WAIT_MS, null))]);
  const out = {};
  if (r) {
    out.graphicsAcceleration = r.rendering;
    if (r.renderer) out.gpu = r.renderer;
    out.videoDecoding = r.video_decode;
  }
  if (win) {
    const d = screen.getDisplayMatching(win.getBounds());
    const w = Math.round(d.size.width * d.scaleFactor);
    const h = Math.round(d.size.height * d.scaleFactor);
    const hz = Math.round(d.displayFrequency || 0);
    out.display = `${OUTPUT}, ${w} × ${h}${hz ? `, ${hz} Hz` : ''}`;
  }
  return out;
}

// Device schedule: the device app blanks the screen at the scheduled times;
// the agent also puts the display in standby (HDMI-CEC or DDC/CI).
function sendTimers(timers) {
  agent.call('set_timers', { timers }).catch((err) => log('schedule not sent to the agent:', err.message));
}
agent.on('disconnect', () => {
  log('agent connection lost');
  if (current.view === 'status') showStatus(lastStatus);
});

// Frame heartbeat: requestAnimationFrame callbacks in the renderer prove that
// frames are being produced; forwarded to the agent (health check, watchdog).
ipcMain.on('castmill:frame', (e) => {
  if (win && e.sender === win.webContents) agent.send({ type: 'frame', output: OUTPUT });
});

// castmillOS bridge: Machine calls from the Castmill device app.
ipcMain.handle('castmill:machine', async (e, method, args) => {
  const frame = e.senderFrame;
  if (!win || e.sender !== win.webContents || !frame || frame !== win.webContents.mainFrame ||
      current.view !== 'content' || !isTrustedBridgeSender(frame.url, current.url, devImage())) {
    log('rejected bridge call from', frame?.url);
    throw new Error('castmillOS: caller not allowed');
  }
  try {
    return await bridge.handle(String(method), Array.isArray(args) ? args : []);
  } catch (err) {
    log(`bridge ${method} failed:`, err.message);
    throw err;
  }
});

app.whenReady().then(() => {
  powerSaveBlocker.start('prevent-display-sleep');
  // Deny all permission requests except what signage content needs.
  const allowed = new Set(['fullscreen', 'clipboard-sanitized-write']);
  session.defaultSession.setPermissionRequestHandler((_wc, perm, cb) => cb(allowed.has(perm)));
  session.defaultSession.setPermissionCheckHandler((_wc, perm) => allowed.has(perm));
  if (process.env.CASTMILL_PROXY_CA) log('note: custom proxy CA is not yet imported into Chromium (TODO)');
  bridge = new MachineBridge({
    dir: app.getPath('userData'),
    agentCall: (method, args) => agent.call(method, args),
    versions: process.versions,
    restart: restartPlayer,
    clearCache: () => session.defaultSession.clearCache(),
    timersChanged: sendTimers,
    hardware: shellHardware,
  });
  createWindow();
  showStatus({ reason: 'starting' });
  agent.start();
  reportGPU();
});

// The process lives as long as its window; systemd restarts it.
app.on('window-all-closed', () => app.quit());
// Stop (systemd signals only this process, KillMode=mixed): save storage and
// exit at once; Electron ends its helper processes.
process.on('SIGTERM', () => {
  agent.close();
  session.defaultSession?.flushStorageData();
  app.exit(0);
});
