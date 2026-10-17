import test from 'node:test';
import assert from 'node:assert/strict';

const stored = new Map([['freenet.tokens', JSON.stringify({ accessToken: 'tok', refreshToken: 'ref' })]]);
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key) => stored.get(key) ?? null,
    setItem: (key, value) => stored.set(key, String(value)),
    removeItem: (key) => stored.delete(key),
  },
});

// sent are the requests the client made; answer says what the server
// answers to each.
let sent = [];
let answer = () => ({ status: 200, body: {} });

globalThis.fetch = async (path, init) => {
  const request = { path, method: init.method, auth: init.headers.Authorization, body: init.body && JSON.parse(init.body) };
  sent.push(request);

  const { status, body } = answer(request);
  return { status, ok: status >= 200 && status < 300, json: async () => body };
};

const { api, idle } = await import('../static/api.js');

function reset(answering) {
  sent = [];
  answer = answering;
}

test('reads made at once go to the server in one batch', async () => {
  reset(({ path, body }) => {
    assert.equal(path, '/api/v1/batch');
    return {
      status: 200,
      body: {
        responses: body.requests.map((r) => (r.path === '/api/v1/me'
          ? { status: 200, body: { id: 'u1' } }
          : { status: 404, body: { error: { code: 'not_found', message: 'post not found' } } })),
      },
    };
  });

  const [me, post] = await Promise.allSettled([api.me(), api.post('p1')]);

  assert.equal(sent.length, 1);
  assert.equal(sent[0].auth, 'Bearer tok');
  assert.deepEqual(sent[0].body, { requests: [{ path: '/api/v1/me' }, { path: '/api/v1/posts/p1' }] });
  assert.deepEqual(me.value, { id: 'u1' });
  assert.equal(post.reason.status, 404);
  assert.equal(post.reason.message, 'post not found');
});

test('a read alone goes by itself, and a change never waits for a batch', async () => {
  reset(() => ({ status: 200, body: { ok: true } }));

  await api.me();
  await api.updateStatus('hi');

  assert.deepEqual(sent.map((r) => `${r.method} ${r.path}`), ['GET /api/v1/me', 'PUT /api/v1/me/status']);
});

test('without batches on the server each read goes by itself', async () => {
  reset(({ path }) => (path === '/api/v1/batch' ? { status: 404, body: null } : { status: 200, body: { path } }));

  const [me, post] = await Promise.all([api.me(), api.post('p1')]);

  assert.deepEqual(sent.map((r) => r.path), ['/api/v1/batch', '/api/v1/me', '/api/v1/posts/p1']);
  assert.equal(me.path, '/api/v1/me');
  assert.equal(post.path, '/api/v1/posts/p1');
});

test('idle waits for the reads the answers start', async () => {
  reset(() => ({ status: 200, body: {} }));

  const order = [];
  api.me().then(() => api.post('p1')).then(() => order.push('read'));
  await idle();
  order.push('idle');

  assert.deepEqual(order, ['read', 'idle']);
});
