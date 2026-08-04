// Mock OpenAI 兼容上游,用于本地测试网关转发链路
const http = require('http');

const MODELS = ['gpt-4o', 'gpt-4o-mini', 'deepseek-chat', 'claude-3-5-sonnet'];

const server = http.createServer((req, res) => {
  let url = new URL(req.url, 'http://localhost');
  // 兼容 base_url 含/不含 /v1 两种形态
  if (url.pathname.startsWith('/v1/')) {
    url = new URL(url.pathname.slice(4), 'http://localhost');
  }
  const auth = req.headers['authorization'] || req.headers['x-api-key'] || '';

  // /models(兼容 /v1/models)
  if (req.method === 'GET' && url.pathname === '/models') {
    if (!auth) { res.writeHead(401, { 'content-type': 'application/json' }); return res.end(JSON.stringify({ error: 'missing key' })); }
    res.writeHead(200, { 'content-type': 'application/json' });
    return res.end(JSON.stringify({ object: 'list', data: MODELS.map(id => ({ id, object: 'model', owned_by: 'mock' })) }));
  }

  // /chat/completions(兼容 /v1/chat/completions)
  if (req.method === 'POST' && url.pathname === '/chat/completions') {
    if (!auth) { res.writeHead(401, { 'content-type': 'application/json' }); return res.end(JSON.stringify({ error: 'missing key' })); }
    let body = '';
    req.on('data', c => body += c);
    req.on('end', () => {
      let payload = {};
      try { payload = JSON.parse(body); } catch (e) {}
      const model = payload.model || 'unknown';
      const stream = !!payload.stream;

      // 模拟失败:model 以 fail- 开头时返回 500
      if (model.startsWith('fail-')) {
        res.writeHead(500, { 'content-type': 'application/json' });
        return res.end(JSON.stringify({ error: { message: 'simulated upstream failure' } }));
      }
      // 模拟 429
      if (model.startsWith('ratelimit-')) {
        res.writeHead(429, { 'content-type': 'application/json' });
        return res.end(JSON.stringify({ error: { message: 'rate limit reached' } }));
      }
      // 模拟业务错误 400
      if (model.startsWith('bizerr-')) {
        res.writeHead(400, { 'content-type': 'application/json' });
        return res.end(JSON.stringify({ error: { message: 'bad request from client' } }));
      }

      const usage = { prompt_tokens: 120, completion_tokens: 45, total_tokens: 165 };

      if (stream) {
        res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache' });
        res.write(`data: ${JSON.stringify({ id: 'chatcmpl-mock', object: 'chat.completion.chunk', model, choices: [{ index: 0, delta: { role: 'assistant', content: '' } }] })}\n\n`);
        res.write(`data: ${JSON.stringify({ id: 'chatcmpl-mock', object: 'chat.completion.chunk', model, choices: [{ index: 0, delta: { content: 'Hello' } }] })}\n\n`);
        res.write(`data: ${JSON.stringify({ id: 'chatcmpl-mock', object: 'chat.completion.chunk', model, choices: [{ index: 0, delta: { content: ' world' } }] })}\n\n`);
        res.write(`data: ${JSON.stringify({ id: 'chatcmpl-mock', object: 'chat.completion.chunk', model, choices: [{ index: 0, delta: {}, finish_reason: 'stop' }], usage })}\n\n`);
        res.write('data: [DONE]\n\n');
        return res.end();
      }

      res.writeHead(200, { 'content-type': 'application/json' });
      return res.end(JSON.stringify({
        id: 'chatcmpl-mock', object: 'chat.completion', created: Math.floor(Date.now() / 1000), model,
        choices: [{ index: 0, message: { role: 'assistant', content: 'Hello from mock upstream!' }, finish_reason: 'stop' }],
        usage,
      }));
    });
    return;
  }

  res.writeHead(404, { 'content-type': 'application/json' });
  res.end(JSON.stringify({ error: 'not found' }));
});

server.listen(9000, () => console.log('mock upstream listening on :9000'));
