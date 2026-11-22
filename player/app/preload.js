'use strict';
// Preload (isolated world, sandboxed). Runs for every page.
//
// - Reports a frame heartbeat to the main process.
// - Exposes window.castmillOS (bridge version 1) to web pages: the Castmill
//   device app uses it as its Machine (docs/castmill-integration.md). The main
//   process answers only the main frame of the Castmill origin.
// - Gives the local status page (file://) read-only state updates.

const { ipcRenderer, contextBridge } = require('electron');

// Frame heartbeat: at most every 2 s, only while frames are being produced.
let last = 0;
function tick(ts) {
  if (ts - last >= 2000) {
    last = ts;
    ipcRenderer.send('castmill:frame');
  }
  requestAnimationFrame(tick);
}
requestAnimationFrame(tick);

if (location.protocol === 'https:' || location.protocol === 'http:') {
  contextBridge.exposeInMainWorld(
    'castmillOS',
    Object.freeze({
      version: 1,
      call: (method, ...args) => ipcRenderer.invoke('castmill:machine', String(method), args),
    }),
  );
}

if (location.protocol === 'file:') {
  contextBridge.exposeInMainWorld('castmillKiosk', {
    onState: (cb) => ipcRenderer.on('castmill:state', (_e, s) => cb(s)),
  });
}
