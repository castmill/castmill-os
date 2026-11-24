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

// ---- On-screen Wi-Fi setup (only while Castmill can't be reached) ----------

const wifi = { visible: false, selected: null, timer: null, busy: false };

function bars(signal) {
  return signal >= 70 ? '▂▄▆█' : signal >= 45 ? '▂▄▆' : signal >= 20 ? '▂▄' : '▂';
}

function wifiMessage(text) {
  $('wifi-message').textContent = text;
}

function selectNetwork(net) {
  wifi.selected = net; // null: "Other network"
  $('wifi-form').hidden = false;
  $('wifi-ssid-label').hidden = net !== null;
  $('wifi-psk-label').hidden = net !== null && !net.secure;
  $('wifi-ssid').value = '';
  $('wifi-psk').value = '';
  wifiMessage(net ? `Selected: ${net.ssid}` : '');
  (net === null ? $('wifi-ssid') : net.secure ? $('wifi-psk') : $('wifi-join')).focus();
}

function renderNetworks(networks) {
  const list = $('wifi-networks');
  list.replaceChildren();
  for (const net of [...networks, null]) {
    const li = document.createElement('li');
    const b = document.createElement('button');
    b.type = 'button';
    b.textContent = net
      ? `${net.ssid}  ${bars(net.signal)}${net.secure ? '  🔒' : ''}${net.active ? '  (connected)' : ''}`
      : 'Other network…';
    b.addEventListener('click', () => selectNetwork(net));
    li.appendChild(b);
    list.appendChild(li);
  }
}

async function refreshWiFi() {
  if (!wifi.visible || wifi.busy || !$('wifi-form').hidden) return;
  try {
    const s = await window.castmillKiosk.wifiStatus();
    if (!s.available) {
      $('wifi').hidden = true;
      return;
    }
    $('wifi').hidden = s.online;
    renderNetworks(s.networks || []);
    if (!s.networks?.length) wifiMessage('Searching for networks…');
  } catch (e) {
    wifiMessage(String(e.message || e).replace(/^Error invoking remote method '[^']+': (Error: )?/, ''));
  }
}

function showWiFi(show) {
  if (show === wifi.visible) return;
  wifi.visible = show;
  clearInterval(wifi.timer);
  if (!show) {
    $('wifi').hidden = true;
    return;
  }
  void refreshWiFi();
  wifi.timer = setInterval(refreshWiFi, 20_000);
}

$('wifi-cancel').addEventListener('click', () => {
  $('wifi-form').hidden = true;
  wifiMessage('');
  void refreshWiFi();
});

$('wifi-form').addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const net = wifi.selected;
  const ssid = net ? net.ssid : $('wifi-ssid').value.trim();
  const psk = $('wifi-psk').value;
  if (!ssid) {
    wifiMessage('Enter the network name.');
    return;
  }
  wifi.busy = true;
  $('wifi-join').disabled = true;
  wifiMessage(`Connecting to ${ssid}…`);
  try {
    await window.castmillKiosk.wifiConnect(ssid, psk, net === null);
    wifiMessage(`Connected to ${ssid}. Reaching Castmill…`);
    $('wifi-form').hidden = true;
  } catch (e) {
    wifiMessage(String(e.message || e).replace(/^Error invoking remote method '[^']+': (Error: )?/, ''));
  } finally {
    wifi.busy = false;
    $('wifi-join').disabled = false;
  }
});

window.castmillKiosk?.onState((s) => showWiFi(s.reason === 'offline' && !!s.agentConnected));
