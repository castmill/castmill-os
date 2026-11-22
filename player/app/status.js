'use strict';
// Local status page, shown while the Castmill device app can't be loaded.
// Receives state from the main process (preload exposes window.castmillKiosk).

const $ = (id) => document.getElementById(id);

const HEADLINES = {
  starting: ['Starting…', ''],
  'invalid-player-url': ['Configuration error', 'The Castmill address configured on this device is not allowed.'],
  offline: ['Cannot reach Castmill', 'Retrying automatically. Check the network connection.'],
};

function render(s) {
  const a = s.agent || {};
  let [headline, detail] = HEADLINES[s.reason] || ['', ''];
  if (!s.agentConnected) {
    headline = 'Starting…';
    detail = 'Waiting for the device agent.';
  }
  if (s.detail) detail = s.detail;
  if (s.reason === 'offline' && a.backend_error) detail = a.backend_error;
  $('headline').textContent = headline;
  $('detail').textContent = detail;
  $('device').textContent = a.device_name || '–';
  $('output').textContent = s.output || '–';
  $('version').textContent = a.os_version || '–';
  $('online').textContent = a.online ? 'reachable' : 'not reachable';
  $('dev').hidden = !a.dev_image;
}

window.castmillKiosk?.onState(render);
