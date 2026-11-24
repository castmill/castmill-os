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
  'getHardwareInfo',
  'setOsUpdate',
  'getOsUpdateStatus',
  'getTelemetry',
  'reboot',
  'restart',
  'clearCache',
  'getWifiStatus',
  'connectWifi',
]);

/** Joining a network takes up to 25 s in the agent. */
const WIFI_CONNECT_TIMEOUT_MS = 35_000;

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
   * @param {(method: string, args?: object, timeoutMs?: number) => Promise<unknown>} o.agentCall  castmill-agent call
   * @param {Record<string,string>} o.versions  process.versions
   * @param {() => void} o.restart  restart this player
   * @param {() => Promise<void>} o.clearCache  clear the browser's HTTP cache
   * @param {(timers: string) => void} [o.timersChanged]  the device schedule (TIMERS) changed
   * @param {() => Promise<object>} [o.hardware]  GPU and display facts from the kiosk shell
   */
  constructor({ dir, agentCall, versions, restart, clearCache, timersChanged, hardware }) {
    this.dir = dir;
    this.agentCall = agentCall;
    this.versions = versions;
    this.restartFn = restart;
    this.clearCacheFn = clearCache;
    this.timersChangedFn = timersChanged;
    this.hardwareFn = hardware;
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
        const changed = s[key] !== value;
        s[key] = value;
        writeAtomic(this.settingsFile, JSON.stringify(s), 0o600);
        if (key === 'TIMERS' && changed) this.timersChangedFn?.(value);
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
      // Separate from getDeviceInfo: Castmill servers reject device info with
      // keys they don't know, so only device apps that know these fields ask.
      case 'getHardwareInfo': {
        const [os, shell] = await Promise.all([
          this.agentCall('hardware_info').catch(() => ({})),
          this.hardwareFn ? this.hardwareFn().catch(() => ({})) : {},
        ]);
        return hardwareInfo({ ...os, ...shell });
      }
      // OS updates from Castmill (ADR-0010): the target version Castmill sent
      // on the device channel, and the agent's progress for the dashboard.
      case 'setOsUpdate': {
        const [target] = args;
        if (target === null || typeof target !== 'object' || Array.isArray(target)) {
          throw new Error('OS update target must be an object');
        }
        await this.agentCall('set_os_update', target);
        return null;
      }
      case 'getOsUpdateStatus':
        return this.agentCall('os_update_status');
      // Wi-Fi from the device app's menu, for a device playing from its cache
      // while offline. The agent only changes the network while offline.
      case 'getWifiStatus':
        return this.agentCall('wifi_status');
      case 'connectWifi': {
        const [ssid, psk, hidden] = args;
        if (typeof ssid !== 'string' || ssid.length === 0 || ssid.length > 32) {
          throw new Error('ssid must be a string of 1 to 32 characters');
        }
        if (typeof psk !== 'string' || psk.length > 64) throw new Error('psk must be a string up to 64 characters');
        if (hidden !== undefined && typeof hidden !== 'boolean') throw new Error('hidden must be a boolean');
        await this.agentCall('wifi_connect', { ssid, psk, hidden: !!hidden }, WIFI_CONNECT_TIMEOUT_MS);
        return null;
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

// Only the documented fields, as strings (videoDecoding: list of strings).
function hardwareInfo(h) {
  const out = {};
  for (const k of ['cpu', 'memory', 'gpu', 'graphicsAcceleration', 'display']) {
    if (typeof h[k] === 'string' && h[k] !== '') out[k] = h[k].slice(0, 200);
  }
  if (Array.isArray(h.videoDecoding)) {
    out.videoDecoding = h.videoDecoding.filter((c) => typeof c === 'string').slice(0, 16);
  }
  return out;
}

module.exports = { MachineBridge, METHODS, SETTING_KEYS };
