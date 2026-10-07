// Synthetic regression probes, executed only inside the network-disabled Linux smoke container.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const crypto = require('node:crypto');
const root = '/usr/local/lib/node_modules/npm/node_modules/';
const { expand } = require(root + 'brace-expansion');
const { WebSocket, request } = require(root + 'undici');

assert.equal(require(root + 'brace-expansion/package.json').version, '5.0.11');
assert.equal(require(root + 'undici/package.json').version, '6.28.1');
assert.equal(JSON.parse(fs.readFileSync('/usr/local/share/claude-runtime/npm-security-overrides.json')).overrides.length, 2);
assert.deepEqual(expand('file-{a,b}-{1..2}'), ['file-a-1', 'file-a-2', 'file-b-1', 'file-b-2']);
// GHSA-6j4f-fj2g-mc7p: parse-side recursion; GHSA-qhr7-859c-m2p7: nested expansion.
for (const pattern of ['{' + '{a},'.repeat(7000) + 'b}', '{'.repeat(3200) + 'a,b' + '}'.repeat(3200)]) {
  assert.doesNotThrow(() => expand(pattern, { max: 1, maxLength: 1 }));
}

(async () => {
  const sockets = new Set();
  const server = http.createServer((req, res) => res.end('synthetic-ok'));
  server.on('connection', socket => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });
  server.on('upgrade', (req, socket) => {
    const accept = crypto.createHash('sha1')
      .update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
    socket.write('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
      'Sec-WebSocket-Accept: ' + accept + '\r\nSec-WebSocket-Protocol: unrequested\r\n\r\n');
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  try {
    const url = '127.0.0.1:' + server.address().port;
    // GHSA-rfgv-xxqx-mfg5: reject a server-selected unrequested protocol without killing Node.
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('WebSocket rejection timed out')), 5000);
      const ws = new WebSocket('ws://' + url);
      ws.addEventListener('error', () => { clearTimeout(timeout); resolve(); }, { once: true });
      ws.addEventListener('open', () => {
        clearTimeout(timeout); ws.close(); reject(new Error('unrequested protocol was accepted'));
      }, { once: true });
    });
    const result = await request('http://' + url, { headersTimeout: 5000, bodyTimeout: 5000 });
    assert.equal(result.statusCode, 200);
    assert.equal(await result.body.text(), 'synthetic-ok');
    console.log('PASS: brace-expansion recursion and undici WebSocket rejection; HTTP client remains usable');
  } finally {
    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
