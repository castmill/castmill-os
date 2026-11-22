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
        try {
          this.emit('message', JSON.parse(line));
        } catch (e) {
          this.emit('warn', `bad message from agent: ${e.message}`);
        }
      }
    });
    const retry = () => {
      if (this.sock === sock) this.sock = null;
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

  close() {
    this.closed = true;
    this.sock?.destroy();
  }
}

module.exports = { AgentLink };
