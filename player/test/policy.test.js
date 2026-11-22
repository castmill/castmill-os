'use strict';
const test = require('node:test');
const assert = require('node:assert');
const { decideView, isNavigationAllowed, outputFromArgv } = require('../app/policy');

test('decideView', () => {
  assert.deepStrictEqual(decideView(null), { view: 'status', reason: 'starting' });
  assert.strictEqual(decideView({ enrolled: false, pairing_code: 'ABC-123' }).reason, 'pairing');
  assert.strictEqual(decideView({ enrolled: true }).reason, 'no-player-url');
  assert.strictEqual(decideView({ enrolled: true, player_url: 'file:///etc/passwd' }).reason, 'invalid-player-url');
  assert.strictEqual(decideView({ enrolled: true, player_url: 'http://evil.example/' }).reason, 'invalid-player-url');
  assert.deepStrictEqual(decideView({ enrolled: true, player_url: 'https://app.castmill.io/p' }),
    { view: 'content', url: 'https://app.castmill.io/p' });
  assert.strictEqual(decideView({ enrolled: true, player_url: 'http://localhost:3000/' }).view, 'content');
});

test('isNavigationAllowed', () => {
  const player = 'https://app.castmill.io/player?d=1';
  assert.ok(isNavigationAllowed('https://app.castmill.io/other', player));
  assert.ok(!isNavigationAllowed('https://evil.example/', player));
  assert.ok(!isNavigationAllowed('file:///etc/passwd', player));
  assert.ok(!isNavigationAllowed('javascript:alert(1)', player));
  assert.ok(!isNavigationAllowed('http://app.castmill.io/', player), 'scheme downgrade');
  assert.ok(isNavigationAllowed('https://cdn.example/x', player, ['https://cdn.example']));
  assert.ok(!isNavigationAllowed('not a url', player));
  assert.ok(!isNavigationAllowed('https://app.castmill.io/', null));
});

test('outputFromArgv', () => {
  assert.strictEqual(outputFromArgv(['electron', 'app', '--castmill-output=HDMI-A-1']), 'HDMI-A-1');
  assert.strictEqual(outputFromArgv(['--castmill-output=../../etc']), process.env.CASTMILL_OUTPUT || 'default');
});
