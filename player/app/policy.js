'use strict';
// Pure decision logic for the kiosk shell (no Electron imports, unit-tested).

/**
 * What the window should show for an agent state.
 * @returns {{view: 'status'|'content', url?: string, reason?: string}}
 */
function decideView(state) {
  if (!state) return { view: 'status', reason: 'starting' };
  if (!state.enrolled) return { view: 'status', reason: 'pairing' };
  if (!state.player_url) return { view: 'status', reason: 'no-player-url' };
  if (!isAllowedScheme(state.player_url)) return { view: 'status', reason: 'invalid-player-url' };
  return { view: 'content', url: state.player_url };
}

function isAllowedScheme(u) {
  try {
    const p = new URL(u);
    return p.protocol === 'https:' || (p.protocol === 'http:' && isLoopback(p.hostname));
  } catch {
    return false;
  }
}

function isLoopback(host) {
  return host === 'localhost' || host === '127.0.0.1' || host === '[::1]';
}

/**
 * Navigation is limited to the player's origin plus explicitly allowed content
 * origins (spec 04, "Least privilege").
 */
function isNavigationAllowed(target, playerURL, extraOrigins = []) {
  let t;
  try {
    t = new URL(target);
  } catch {
    return false;
  }
  if (t.protocol === 'file:') return false; // local pages are loaded by main, never navigated to
  if (!isAllowedScheme(target)) return false;
  const allowed = new Set(extraOrigins);
  try {
    allowed.add(new URL(playerURL).origin);
  } catch {
    /* no player URL yet */
  }
  return allowed.has(t.origin);
}

/** Parses "--output=HDMI-A-1" style arguments. */
function outputFromArgv(argv) {
  for (const a of argv) {
    const m = /^--castmill-output=([A-Za-z0-9_.-]+)$/.exec(a);
    if (m) return m[1];
  }
  return process.env.CASTMILL_OUTPUT || 'default';
}

module.exports = { decideView, isNavigationAllowed, outputFromArgv, isAllowedScheme };
