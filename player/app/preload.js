'use strict';
// Preload (isolated world, sandboxed). Runs for every page, including remote
// player content, so it exposes nothing to remote pages and only reports
// frames and playback stats to the main process.

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

// Playback quality of every <video> element (dropped frames, MGT-004).
setInterval(() => {
  const videos = Array.from(document.querySelectorAll('video'));
  if (videos.length === 0) return;
  let dropped = 0;
  let total = 0;
  for (const v of videos) {
    const q = v.getVideoPlaybackQuality?.();
    if (q) {
      dropped += q.droppedVideoFrames;
      total += q.totalVideoFrames;
    }
  }
  ipcRenderer.send('castmill:stats', { videos: videos.length, dropped_frames: dropped, total_frames: total });
}, 60_000);

// The local status page (file://) gets read-only state updates.
if (location.protocol === 'file:') {
  contextBridge.exposeInMainWorld('castmillKiosk', {
    onState: (cb) => ipcRenderer.on('castmill:state', (_e, s) => cb(s)),
  });
}
