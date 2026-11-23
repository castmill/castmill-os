'use strict';
// GPU report (MED-002 / MED-007): does Chromium render and decode video on
// the GPU? Sent to castmill-agent, which shows it in "castmill-agent status"
// and logs a warning when the player fell back to software rendering.

// Formats checked for hardware decoding. Chromium reports a format as
// "powerEfficient" when its GPU video decoder handles it (VA-API on Intel).
const CODEC_PROBES = [
  { name: 'H.264', contentType: 'video/mp4; codecs="avc1.640028"', width: 1920, height: 1080 },
  { name: 'HEVC', contentType: 'video/mp4; codecs="hvc1.1.6.L153.B0"', width: 3840, height: 2160 },
  { name: 'VP9', contentType: 'video/webm; codecs="vp09.00.51.08"', width: 3840, height: 2160 },
  { name: 'AV1', contentType: 'video/mp4; codecs="av01.0.12M.08"', width: 3840, height: 2160 },
];

// Chromium feature status values that mean "on the GPU" (chrome://gpu).
function onGPU(status) {
  return typeof status === 'string' && status.startsWith('enabled');
}

/**
 * Builds the report for the agent.
 * @param {Record<string,string>} featureStatus app.getGPUFeatureStatus()
 * @param {object} gpuInfo app.getGPUInfo('complete')
 * @param {string[]} decoded names from CODEC_PROBES decoded in hardware
 */
function summarize(featureStatus, gpuInfo, decoded) {
  const features = {};
  for (const [k, v] of Object.entries(featureStatus || {})) {
    if (typeof v === 'string') features[k] = v;
  }
  const aux = gpuInfo?.auxAttributes || {};
  return {
    rendering: onGPU(features.gpu_compositing) ? 'hardware' : 'software',
    renderer: typeof aux.glRenderer === 'string' ? aux.glRenderer : '',
    video_decode: CODEC_PROBES.map((p) => p.name).filter((n) => decoded.includes(n)),
    features,
  };
}

// Runs in a renderer: the formats Chromium decodes in hardware.
// Serialized with toString() and run with executeJavaScript.
/* global navigator */
async function decodeProbe(probes) {
  const out = [];
  for (const p of probes) {
    try {
      const r = await navigator.mediaCapabilities.decodingInfo({
        type: 'media-source',
        video: { contentType: p.contentType, width: p.width, height: p.height, bitrate: 20_000_000, framerate: 30 },
      });
      if (r.supported && r.powerEfficient) out.push(p.name);
    } catch {
      // Unknown format: not decoded in hardware.
    }
  }
  return out;
}

/**
 * Probes the GPU with a hidden window and returns the report.
 * @param {typeof import('electron')} electron
 */
async function probeGPU({ app, BrowserWindow }) {
  const gpuInfo = await app.getGPUInfo('complete').catch(() => ({}));
  const probe = new BrowserWindow({
    show: false,
    webPreferences: { sandbox: true, contextIsolation: true, nodeIntegration: false, javascript: true },
  });
  try {
    await probe.loadURL('about:blank');
    const decoded = await probe.webContents.executeJavaScript(
      `(${decodeProbe.toString()})(${JSON.stringify(CODEC_PROBES)})`,
    );
    return summarize(app.getGPUFeatureStatus(), gpuInfo, Array.isArray(decoded) ? decoded : []);
  } finally {
    probe.destroy();
  }
}

module.exports = { CODEC_PROBES, summarize, decodeProbe, probeGPU };
