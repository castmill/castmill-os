'use strict';
const test = require('node:test');
const assert = require('node:assert');
const { decideView, isNavigationAllowed, isTrustedBridgeSender, outputFromArgv } = require('../app/policy');

test('decideView', () => {
  assert.deepStrictEqual(decideView(null), { view: 'status', reason: 'starting' });
  assert.strictEqual(decideView({ os_version: '0.1.0' }).reason, 'starting');
  assert.strictEqual(decideView({ player_url: 'file:///etc/passwd' }).reason, 'invalid-player-url');
  assert.strictEqual(decideView({ player_url: 'http://castmill.example/' }).reason, 'invalid-player-url');
  assert.deepStrictEqual(decideView({ player_url: 'https://castmill.net/' }), { view: 'content', url: 'https://castmill.net/' });
  assert.strictEqual(decideView({ player_url: 'http://localhost:4000/' }).view, 'content');
  // Development images may use a plain-http Castmill dev server.
  assert.strictEqual(decideView({ player_url: 'http://10.0.2.2:4000/', dev_image: true }).view, 'content');
});

test('isNavigationAllowed', () => {
  const player = 'https://castmill.net/';
  assert.ok(isNavigationAllowed('https://castmill.net/other', player));
  assert.ok(!isNavigationAllowed('https://evil.example/', player));
  assert.ok(!isNavigationAllowed('file:///etc/passwd', player));
  assert.ok(!isNavigationAllowed('javascript:alert(1)', player));
  assert.ok(!isNavigationAllowed('http://castmill.net/', player), 'scheme downgrade');
  assert.ok(isNavigationAllowed('https://cdn.example/x', player, ['https://cdn.example']));
  assert.ok(!isNavigationAllowed('not a url', player));
  assert.ok(!isNavigationAllowed('https://castmill.net/', null));
});

test('isTrustedBridgeSender', () => {
  const player = 'https://castmill.net/';
  assert.ok(isTrustedBridgeSender('https://castmill.net/?x=1', player));
  assert.ok(!isTrustedBridgeSender('https://evil.example/', player));
  assert.ok(!isTrustedBridgeSender('https://castmill.net.evil.example/', player));
  assert.ok(!isTrustedBridgeSender('http://castmill.net/', player), 'scheme must match');
  assert.ok(!isTrustedBridgeSender('https://castmill.net/', null));
  assert.ok(!isTrustedBridgeSender('file:///x', 'file:///x'));
  assert.ok(isTrustedBridgeSender('http://10.0.2.2:4000/', 'http://10.0.2.2:4000/', true));
  assert.ok(!isTrustedBridgeSender('http://10.0.2.2:4000/', 'http://10.0.2.2:4000/', false));
});

test('outputFromArgv', () => {
  assert.strictEqual(outputFromArgv(['electron', 'app', '--castmill-output=HDMI-A-1']), 'HDMI-A-1');
  assert.strictEqual(outputFromArgv(['--castmill-output=../../etc']), process.env.CASTMILL_OUTPUT || 'default');
});
