'use strict';
// Local status page: pairing code, connection problems, device facts.
// Receives state from the main process (preload exposes window.castmillKiosk).

const $ = (id) => document.getElementById(id);

const HEADLINES = {
  starting: ['Starting…', ''],
  pairing: ['Waiting for pairing', ''],
  'no-player-url': ['Paired', 'Waiting for the player configuration from Castmill.'],
  'invalid-player-url': ['Configuration error', 'The player URL from Castmill is not allowed.'],
  offline: ['Cannot reach the player', 'Retrying automatically. Check the network connection.'],
};

function render(s) {
  const a = s.agent || {};
  const pairing = s.reason === 'pairing' && a.pairing_code;
  $('pairing').hidden = !pairing;
  $('code').textContent = a.pairing_code || ' ';
  let [headline, detail] = HEADLINES[s.reason] || ['', ''];
  if (pairing) headline = '';
  if (s.reason === 'pairing' && !a.pairing_code) {
    headline = 'Contacting Castmill…';
  }
  if (!s.agentConnected) {
    headline = 'Device agent not running';
    detail = 'The screen will recover automatically.';
  }
  if (a.backend_error) detail = a.backend_error;
  if (s.detail) detail = s.detail;
  $('headline').textContent = headline;
  $('headline').hidden = !headline;
  $('detail').textContent = detail;
  $('device').textContent = a.device_id || a.fingerprint || '–';
  $('output').textContent = s.output || '–';
  $('version').textContent = a.os_version || '–';
  $('online').textContent = a.online ? 'connected' : 'not connected';
  $('dev').hidden = !a.dev_image;
}

window.castmillKiosk?.onState(render);
