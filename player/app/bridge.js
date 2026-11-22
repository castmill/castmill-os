'use strict';
// The castmillOS bridge: the native side of Castmill's `Machine` interface
// (packages/device/src/interfaces/machine.ts in the Castmill repository) for
// the Castmill device app running in this kiosk shell. No Electron imports
// (unit-tested); main.js wires it to IPC, the agent and the app.
//
// Storage (settings, credentials) lives in this output's profile directory on
// /data; OS facts and actions come from castmill-agent. See
// docs/castmill-integration.md.

const fs = require('node:fs');
const path = require('node:path');

const SETTING_KEYS = new Set(['BASE_URL', 'TIMERS', 'TIMER_OFF', 'ORIENTATION']);
const MAX_VALUE = 64 * 1024;

/** Methods the bridge answers. Others are rejected. */
const METHODS = new Set([
  'getSetting',
  'setSetting',
  'getMachineGUID',
  'storeCredentials',
  'getCredentials',
  'removeCredentials',
  'getTimezone',
  'getDeviceInfo',
  'getTelemetry',
  'reboot',
  'restart',
  'clearCache',
]);

function writeAtomic(file, data, mode) {
  const tmp = `${file}.tmp-${process.pid}`;
  const fd = fs.openSync(tmp, 'w', mode);
  try {
    fs.writeSync(fd, data);
    fs.fsyncSync(fd);
  } finally {
    fs.closeSync(fd);
  }
  fs.renameSync(tmp, file);
}

function checkString(v, what) {
  if (typeof v !== 'string' || v.length > MAX_VALUE) throw new Error(`${what} must be a string up to 64 KiB`);
}

class MachineBridge {
  /**
   * @param {object} o
   * @param {string} o.dir          storage directory (this output's profile)
   * @param {(method: string) => Promise<unknown>} o.agentCall  castmill-agent call
   * @param {Record<string,string>} o.versions  process.versions
   * @param {() => void} o.restart  restart this player
   * @param {() => Promise<void>} o.clearCache  clear the browser's HTTP cache
   */
  constructor({ dir, agentCall, versions, restart, clearCache }) {
    this.dir = dir;
    this.agentCall = agentCall;
    this.versions = versions;
    this.restartFn = restart;
    this.clearCacheFn = clearCache;
    fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  }

  get settingsFile() {
    return path.join(this.dir, 'castmill-settings.json');
  }

  get credentialsFile() {
    return path.join(this.dir, 'castmill-credentials');
  }

  readSettings() {
    try {
      return JSON.parse(fs.readFileSync(this.settingsFile, 'utf8'));
    } catch {
      return {};
    }
  }

  /** Dispatches one bridge call. Throws on unknown methods or bad arguments. */
  async handle(method, args = []) {
    if (!METHODS.has(method)) throw new Error(`unknown method ${method}`);
    if (!Array.isArray(args)) throw new Error('arguments must be an array');
    switch (method) {
      case 'getSetting': {
        const [key] = args;
        if (!SETTING_KEYS.has(key)) throw new Error(`unknown setting ${key}`);
        return this.readSettings()[key] ?? null;
      }
      case 'setSetting': {
        const [key, value] = args;
        if (!SETTING_KEYS.has(key)) throw new Error(`unknown setting ${key}`);
        checkString(value, 'value');
        const s = this.readSettings();
        s[key] = value;
        writeAtomic(this.settingsFile, JSON.stringify(s), 0o600);
        return null;
      }
      case 'storeCredentials':
        checkString(args[0], 'credentials');
        writeAtomic(this.credentialsFile, args[0], 0o600);
        return null;
      case 'getCredentials':
        try {
          return fs.readFileSync(this.credentialsFile, 'utf8');
        } catch {
          return null;
        }
      case 'removeCredentials':
        fs.rmSync(this.credentialsFile, { force: true });
        return null;
      case 'getMachineGUID':
        return this.agentCall('machine_id');
      case 'getTimezone':
        return this.agentCall('timezone');
      case 'getTelemetry':
        return this.agentCall('telemetry');
      case 'reboot':
        return this.agentCall('reboot');
      case 'getDeviceInfo': {
        const info = await this.agentCall('device_info');
        const v = this.versions;
        return {
          ...info,
          environmentVersion: `Electron ${v.electron}`,
          chromiumVersion: v.chrome,
          v8Version: v.v8,
          nodeVersion: v.node,
        };
      }
      case 'restart':
        setImmediate(this.restartFn);
        return null;
      case 'clearCache':
        await this.clearCacheFn();
        return null;
    }
    throw new Error(`unhandled method ${method}`);
  }
}

module.exports = { MachineBridge, METHODS, SETTING_KEYS };
