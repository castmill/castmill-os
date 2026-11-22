'use strict';
// Client side of the agent <-> player link (see agent/internal/playerlink).
// Newline-delimited JSON over a Unix socket, reconnecting forever: the player
// must keep running when the agent restarts.

const net = require('node:net');
const { EventEmitter } = require('node:events');

class AgentLink extends EventEmitter {
  constructor(socketPath, output, { retryMs = 2000 } = {}) {
    super();
    this.socketPath = socketPath;
    this.output = output;
    this.retryMs = retryMs;
    this.sock = null;
    this.closed = false;
    this.connected = false;
    this.pending = new Map(); // call id -> {resolve, reject, timer}
    this.seq = 0;
  }

  start() {
    this._connect();
    return this;
  }

  _connect() {
    if (this.closed) return;
    const sock = net.createConnection(this.socketPath);
    let buf = '';
    sock.setEncoding('utf8');
    sock.on('connect', () => {
      this.sock = sock;
      this.connected = true;
      this.send({ type: 'hello', output: this.output });
      this.emit('connect');
    });
    sock.on('data', (chunk) => {
      buf += chunk;
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i);
        buf = buf.slice(i + 1);
        if (!line) continue;
        let msg;
        try {
          msg = JSON.parse(line);
        } catch (e) {
          this.emit('warn', `bad message from agent: ${e.message}`);
          continue;
        }
        if (msg.type === 'reply') this._settle(msg);
        else this.emit('message', msg);
      }
    });
    const retry = () => {
      if (this.sock === sock) this.sock = null;
      this._failPending(new Error('agent connection lost'));
      if (this.connected) this.emit('disconnect');
      this.connected = false;
      if (!this.closed) setTimeout(() => this._connect(), this.retryMs).unref?.();
    };
    sock.on('error', () => {}); // 'close' follows
    sock.on('close', retry);
  }

  send(msg) {
    if (!this.sock) return false;
    this.sock.write(JSON.stringify(msg) + '\n');
    return true;
  }

  /**
   * Calls an agent method (see agent/internal/playerlink). Resolves with the
   * reply data, rejects on an agent error, a lost connection or a timeout.
   */
  call(method, args = {}, timeoutMs = 30_000) {
    return new Promise((resolve, reject) => {
      const id = String(++this.seq);
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`agent call ${method} timed out`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      if (!this.send({ type: 'call', id, method, args })) {
        clearTimeout(timer);
        this.pending.delete(id);
        reject(new Error('agent not connected'));
      }
    });
  }

  _settle(msg) {
    const p = this.pending.get(msg.id);
    if (!p) return;
    this.pending.delete(msg.id);
    clearTimeout(p.timer);
    if (msg.ok) p.resolve(msg.data ?? null);
    else p.reject(new Error(msg.error || 'agent call failed'));
  }

  _failPending(err) {
    for (const [id, p] of this.pending) {
      clearTimeout(p.timer);
      p.reject(err);
      this.pending.delete(id);
    }
  }

  close() {
    this.closed = true;
    this._failPending(new Error('closed'));
    this.sock?.destroy();
  }
}

module.exports = { AgentLink };
