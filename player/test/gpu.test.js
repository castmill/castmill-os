'use strict';
const test = require('node:test');
const assert = require('node:assert');
const { CODEC_PROBES, summarize, decodeProbe } = require('../app/gpu');

const intelInfo = { auxAttributes: { glRenderer: 'Mesa Intel(R) Graphics (ADL-N)' } };

test('hardware rendering with hardware video decoding', () => {
  const r = summarize(
    { gpu_compositing: 'enabled', rasterization: 'enabled', video_decode: 'enabled', webgl: 'enabled' },
    intelInfo,
    ['AV1', 'H.264', 'HEVC', 'VP9'],
  );
  assert.deepStrictEqual(r, {
    rendering: 'hardware',
    renderer: 'Mesa Intel(R) Graphics (ADL-N)',
    video_decode: ['H.264', 'HEVC', 'VP9', 'AV1'], // in probe order
    features: { gpu_compositing: 'enabled', rasterization: 'enabled', video_decode: 'enabled', webgl: 'enabled' },
  });
});

test('software fallback', () => {
  const r = summarize({ gpu_compositing: 'disabled_software', video_decode: 'unavailable_software' }, {}, []);
  assert.strictEqual(r.rendering, 'software');
  assert.strictEqual(r.renderer, '');
  assert.deepStrictEqual(r.video_decode, []);
});

test('missing or odd inputs', () => {
  const r = summarize(undefined, undefined, ['Theora']);
  assert.deepStrictEqual(r, { rendering: 'software', renderer: '', video_decode: [], features: {} });
  assert.strictEqual(summarize({ gpu_compositing: 'enabled_on', x: 1 }, {}, []).rendering, 'hardware');
  assert.deepStrictEqual(summarize({ gpu_compositing: 'enabled_on', x: 1 }, {}, []).features, { gpu_compositing: 'enabled_on' });
});

test('decode probe keeps power-efficient formats only', async () => {
  const answers = {
    'H.264': { supported: true, powerEfficient: true },
    HEVC: { supported: true, powerEfficient: false }, // software decoder
    VP9: { supported: false, powerEfficient: false },
  };
  const saved = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  const navigator = {
    mediaCapabilities: {
      decodingInfo: async (cfg) => {
        const p = CODEC_PROBES.find((x) => x.contentType === cfg.video.contentType);
        if (p.name === 'AV1') throw new TypeError('unsupported');
        return answers[p.name];
      },
    },
  };
  Object.defineProperty(globalThis, 'navigator', { value: navigator, configurable: true });
  try {
    assert.deepStrictEqual(await decodeProbe(CODEC_PROBES), ['H.264']);
  } finally {
    if (saved) Object.defineProperty(globalThis, 'navigator', saved);
    else delete globalThis.navigator;
  }
});
