import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname, sep } from 'node:path';
const root = resolve('dist');
const mime = { '.html':'text/html; charset=utf-8', '.js':'text/javascript', '.css':'text/css', '.svg':'image/svg+xml', '.png':'image/png' };
const server = createServer(async (req,res) => {
  if (req.method === 'GET' && req.url === '/__preview_stop') {
    res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' }).end('Preview session stopped.');
    server.close();
    server.closeIdleConnections();
    return;
  }
  try {
    const pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
    const file = resolve(root, '.' + (pathname === '/' ? '/index.html' : pathname));
    if (!file.startsWith(root + sep)) { res.writeHead(403).end(); return; }
    const data = await readFile(file);
    res.writeHead(200, { 'Content-Type': mime[extname(file)] ?? 'application/octet-stream', 'Cache-Control':'no-store' });
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
server.listen(4179, '127.0.0.1', () => console.log('Krasis preview: http://127.0.0.1:4179 (10 minute bounded session)'));
const timer = setTimeout(() => server.close(), 600_000);
server.on('close', () => clearTimeout(timer));
