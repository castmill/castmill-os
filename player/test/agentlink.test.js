'use strict';
const test = require('node:test');
const assert = require('node:assert');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const fs = require('node:fs');
const { once } = require('node:events');
const { AgentLink } = require('../app/agentlink');

function server(sockPath) {
  const lines = [];
  const conns = [];
  const srv = net.createServer((c) => {
    conns.push(c);
    let buf = '';
    c.setEncoding('utf8');
    c.on('data', (d) => {
      buf += d;
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        lines.push(JSON.parse(buf.slice(0, i)));
        buf = buf.slice(i + 1);
        srv.emit('line');
      }
    });
  });
  srv.listen(sockPath);
  return { srv, lines, conns };
}

test('hello, messages both ways, reconnect after agent restart', async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cmk-'));
  const sock = path.join(dir, 's');
  let s = server(sock);
  await once(s.srv, 'listening');

  const link = new AgentLink(sock, 'HDMI-A-1', { retryMs: 50 }).start();
  await once(s.srv, 'line');
  assert.deepStrictEqual(s.lines[0], { type: 'hello', output: 'HDMI-A-1' });

  const got = once(link, 'message');
  s.conns[0].write('{"type":"state","state":{"enrolled":true}}\n{"type":"reload"}\n');
  assert.deepStrictEqual((await got)[0], { type: 'state', state: { enrolled: true } });

  // Agent restarts: the link reconnects and says hello again.
  s.conns[0].destroy();
  s.srv.close();
  fs.rmSync(sock, { force: true });
  await once(link, 'disconnect');
  s = server(sock);
  await once(s.srv, 'line');
  assert.deepStrictEqual(s.lines[0], { type: 'hello', output: 'HDMI-A-1' });
  assert.ok(link.send({ type: 'frame', output: 'HDMI-A-1' }));

  // call(): replies settle the matching promise; errors reject.
  s.srv.once('line', () => {});
  const p1 = link.call('timezone');
  const p2 = link.call('reboot');
  await new Promise((r) => setTimeout(r, 50));
  const calls = s.lines.filter((l) => l.type === 'call');
  assert.deepStrictEqual(calls.map((c) => c.method), ['timezone', 'reboot']);
  s.conns[0].write(JSON.stringify({ type: 'reply', id: calls[1].id, error: 'reboot not available' }) + '\n');
  s.conns[0].write(JSON.stringify({ type: 'reply', id: calls[0].id, ok: true, data: 'Europe/Madrid' }) + '\n');
  assert.strictEqual(await p1, 'Europe/Madrid');
  await assert.rejects(p2, /reboot not available/);
  await assert.rejects(link.call('telemetry', {}, 50), /timed out/);

  link.close();
  s.srv.close();
  fs.rmSync(dir, { recursive: true, force: true });
});
