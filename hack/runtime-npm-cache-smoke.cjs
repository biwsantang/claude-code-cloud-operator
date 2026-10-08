// Exercise the installed npm cache caller, not a replacement implementation.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const root = '/usr/local/lib/node_modules/npm/node_modules/';
const CachePolicy = require(root + 'make-fetch-happen/lib/cache/policy.js');
const { Request, Response } = require(root + 'minipass-fetch');
const fetch = require(root + 'make-fetch-happen');

(async () => {
  const request = new Request('https://synthetic.invalid/package');
  const response = new Response('synthetic', {
    headers: { 'set-cookie': 'synthetic-session=a', 'cache-control': 'public, max-age=3600' },
  });
  const policy = new CachePolicy({ request, response, options: {} });
  assert.equal(policy.policy._isShared, false, 'npm must use a private cache policy');
  assert.equal(CachePolicy.storable(request, { cachePath: '/tmp/synthetic', cache: 'no-store' }), false);

  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'npm-cache-review-'));
  const sockets = new Set();
  let requests = 0;
  const server = http.createServer((req, res) => {
    requests++;
    res.writeHead(200, { 'cache-control': 'public, max-age=3600', 'set-cookie': 'synthetic-session=a' });
    res.end('synthetic-response-' + requests);
  });
  server.on('connection', socket => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });
  try {
    await new Promise((resolve, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', resolve);
    });
    const url = 'http://127.0.0.1:' + server.address().port + '/package';
    const options = { cachePath: path.join(directory, 'a'), cache: 'force-cache', retry: 0, timeout: 5000 };
    const first = await fetch(url, options);
    assert.equal(first.headers.get('set-cookie'), 'synthetic-session=a');
    assert.equal(await first.text(), 'synthetic-response-1');
    const cached = await fetch(url, { ...options, headers: { 'cache-control': 'max-stale=999999' } });
    assert.equal(await cached.text(), 'synthetic-response-1');
    assert.equal(cached.headers.get('set-cookie'), null, 'default npm cache must not retain Set-Cookie');
    assert.equal(requests, 1, 'positive control must actually hit the cache');
    const other = await fetch(url, { ...options, cachePath: path.join(directory, 'b') });
    assert.equal(await other.text(), 'synthetic-response-2');
    assert.equal(requests, 2, 'a separate cache must not reuse the first session response');
    console.log('PASS: installed npm private cache policy, no-store, Set-Cookie omission and separate cache paths');
  } finally {
    for (const socket of sockets) socket.destroy();
    if (server.listening) await new Promise(resolve => server.close(resolve));
    await fs.rm(directory, { recursive: true, force: true });
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
