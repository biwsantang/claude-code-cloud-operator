// Synthetic regression probes, executed only inside the network-disabled Linux smoke container.
const assert = require('node:assert/strict');
const http = require('node:http');
const crypto = require('node:crypto');
const root = '/usr/local/lib/node_modules/npm/node_modules/';
const brace = require(root + 'brace-expansion');
const expand = typeof brace === 'function' ? brace : brace.expand;
const { WebSocket, request } = require(root + 'undici');

console.log('npm bundle:', { braceExpansion: require(root + 'brace-expansion/package.json').version,
  undici: require(root + 'undici/package.json').version });
const probes = ['normal-braces', 'parse-recursion', 'nested-recursion', 'websocket'];
const selected = process.argv[2];
if (!selected) {
  const { spawnSync } = require('node:child_process');
  let failed = false;
  for (const probe of probes) {
    const result = spawnSync(process.execPath, [__filename, probe], { stdio: 'inherit', timeout: 15000 });
    if (result.error || result.status !== 0) {
      console.error('FAIL:', probe, result.error || result.signal || result.status);
      failed = true;
    }
  }
  process.exit(failed ? 1 : 0);
}
assert(probes.includes(selected), 'unknown security probe');
if (selected !== 'websocket') {
  if (selected === 'normal-braces') {
    assert.deepEqual(expand('file-{a,b}-{1..2}'), ['file-a-1', 'file-a-2', 'file-b-1', 'file-b-2']);
  } else {
    // Each advisory runs in its own process so a crash cannot skip later probes.
    const pattern = selected === 'parse-recursion' ? '{' + '{a},'.repeat(7000) + 'b}' :
      '{'.repeat(3200) + 'a,b' + '}'.repeat(3200);
    assert.doesNotThrow(() => expand(pattern, { max: 1, maxLength: 1 }));
  }
  console.log('PASS:', selected);
  process.exit(0);
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
    console.log('PASS: undici WebSocket rejection; HTTP client remains usable');
  } finally {
    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
