'use strict';
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { MachineBridge } = require('../app/bridge');

function bridge(overrides = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cmb-'));
  const calls = [];
  const b = new MachineBridge({
    dir,
    agentCall: async (m) => {
      calls.push(m);
      if (m === 'device_info') return { appType: 'Castmill OS', appVersion: '0.1.0', os: 'Castmill OS 0.1.0', hardware: 'genericarm64' };
      if (m === 'machine_id') return 'abc-HDMI-A-1';
      return { m };
    },
    versions: { electron: '45.4.1', chrome: '140.0', v8: '14.0', node: '24.22.3' },
    restart: () => calls.push('restart'),
    clearCache: async () => calls.push('clearCache'),
    ...overrides,
  });
  return { b, dir, calls };
}

test('settings: only Castmill keys, persisted', async () => {
  const { b, dir } = bridge();
  assert.strictEqual(await b.handle('getSetting', ['BASE_URL']), null);
  await b.handle('setSetting', ['BASE_URL', 'https://castmill.net']);
  assert.strictEqual(await b.handle('getSetting', ['BASE_URL']), 'https://castmill.net');
  await assert.rejects(b.handle('setSetting', ['../../etc', 'x']), /unknown setting/);
  await assert.rejects(b.handle('setSetting', ['TIMERS', 42]), /string/);
  await assert.rejects(b.handle('setSetting', ['TIMERS', 'x'.repeat(65 * 1024)]), /64 KiB/);
  // A fresh bridge on the same directory (player restart) reads it back.
  const again = new MachineBridge({ dir, agentCall: async () => null, versions: {}, restart() {}, clearCache: async () => {} });
  assert.strictEqual(await again.handle('getSetting', ['BASE_URL']), 'https://castmill.net');
});

test('credentials: private file, removable', async () => {
  const { b, dir } = bridge();
  assert.strictEqual(await b.handle('getCredentials'), null);
  await b.handle('storeCredentials', ['{"device":{"id":"d1","token":"secret"}}']);
  assert.strictEqual(await b.handle('getCredentials'), '{"device":{"id":"d1","token":"secret"}}');
  const mode = fs.statSync(path.join(dir, 'castmill-credentials')).mode & 0o777;
  assert.strictEqual(mode, 0o600);
  await b.handle('removeCredentials');
  assert.strictEqual(await b.handle('getCredentials'), null);
});

test('OS calls go to the agent; device info adds runtime versions', async () => {
  const { b, calls } = bridge();
  assert.strictEqual(await b.handle('getMachineGUID'), 'abc-HDMI-A-1');
  const info = await b.handle('getDeviceInfo');
  assert.deepStrictEqual(info, {
    appType: 'Castmill OS', appVersion: '0.1.0', os: 'Castmill OS 0.1.0', hardware: 'genericarm64',
    environmentVersion: 'Electron 45.4.1', chromiumVersion: '140.0', v8Version: '14.0', nodeVersion: '24.22.3',
  });
  await b.handle('getTelemetry');
  await b.handle('getTimezone');
  await b.handle('reboot');
  assert.deepStrictEqual(calls, ['machine_id', 'device_info', 'telemetry', 'timezone', 'reboot']);
});

test('app-level calls and unknown methods', async () => {
  const { b, calls } = bridge();
  await b.handle('clearCache');
  await b.handle('restart');
  await new Promise((r) => setImmediate(r));
  assert.deepStrictEqual(calls, ['clearCache', 'restart']);
  await assert.rejects(b.handle('shutdown'), /unknown method/);
  await assert.rejects(b.handle('__proto__'), /unknown method/);
  await assert.rejects(b.handle('getSetting', 'BASE_URL'), /array/);
});

test('settings: schedule changes are forwarded once', async () => {
  const sent = [];
  const { b } = bridge({ timersChanged: (t) => sent.push(t) });
  const timers = '{"on":[{"hours":8,"minutes":0,"weekDays":["ALL"]}],"off":[]}';
  await b.handle('setSetting', ['TIMERS', timers]);
  await b.handle('setSetting', ['TIMERS', timers]); // unchanged: not sent again
  await b.handle('setSetting', ['TIMER_OFF', 'true']);
  assert.deepStrictEqual(sent, [timers]);
});

test('hardware info: agent and kiosk shell facts, documented fields only', async () => {
  const { b, calls } = bridge({
    agentCall: async (m) => {
      calls.push(m);
      return { cpu: 'Intel N100 (4 cores)', memory: '8 GB', serial: 'not exposed' };
    },
    hardware: async () => ({
      gpu: 'Mesa Intel(R) Graphics (ADL-N)',
      graphicsAcceleration: 'hardware',
      videoDecoding: ['H.264', 'HEVC', 42],
      display: 'HDMI-A-1, 3840 × 2160, 60 Hz',
      extra: 'dropped',
    }),
  });
  assert.deepStrictEqual(await b.handle('getHardwareInfo'), {
    cpu: 'Intel N100 (4 cores)',
    memory: '8 GB',
    gpu: 'Mesa Intel(R) Graphics (ADL-N)',
    graphicsAcceleration: 'hardware',
    display: 'HDMI-A-1, 3840 × 2160, 60 Hz',
    videoDecoding: ['H.264', 'HEVC'],
  });
  assert.deepStrictEqual(calls, ['hardware_info']);
});

test('hardware info: partial when the agent or the GPU probe fails', async () => {
  const { b } = bridge({
    agentCall: async () => {
      throw new Error('unknown method "hardware_info"'); // older agent
    },
    hardware: async () => ({ display: 'Virtual-1, 1920 × 1080' }),
  });
  assert.deepStrictEqual(await b.handle('getHardwareInfo'), { display: 'Virtual-1, 1920 × 1080' });
  const { b: noShell } = bridge({ agentCall: async () => ({ memory: '4 GB' }) });
  assert.deepStrictEqual(await noShell.handle('getHardwareInfo'), { memory: '4 GB' });
});

test('OS updates: target to the agent, status back', async () => {
  const sent = [];
  const { b } = bridge({
    agentCall: async (m, args) => {
      sent.push([m, args]);
      return m === 'os_update_status' ? { state: 'installing', progress: 40, current_version: '0.2.0' } : null;
    },
  });
  const target = { version: '0.3.0', url: 'https://example.com/b.raucb', activate: 'window' };
  assert.strictEqual(await b.handle('setOsUpdate', [target]), null);
  assert.deepStrictEqual(await b.handle('getOsUpdateStatus'), {
    state: 'installing',
    progress: 40,
    current_version: '0.2.0',
  });
  assert.deepStrictEqual(sent, [
    ['set_os_update', target],
    ['os_update_status', undefined],
  ]);
  await assert.rejects(b.handle('setOsUpdate', ['0.3.0']), /object/);
  await assert.rejects(b.handle('setOsUpdate', []), /object/);
});
