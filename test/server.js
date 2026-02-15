import { createServer } from 'http';
import { readFileSync } from 'fs';
import { WebSocketServer } from 'ws';

const PORT = 3000;

// HTTP 服务器
const server = createServer((req, res) => {
  if (req.url === '/' || req.url === '/index.html') {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(readFileSync('./index.html'));
  } else {
    res.writeHead(404);
    res.end('Not Found');
  }
});

// WebSocket 服务器
const wss = new WebSocketServer({ server });
const clients = new Map();

wss.on('connection', (ws) => {
  const id = Date.now().toString(36);
  clients.set(ws, { id, name: `用户${id}` });
  
  broadcast({ type: 'system', content: `${clients.get(ws).name} 加入了聊天室` });
  console.log(`✅ ${clients.get(ws).name} 已连接`);

  ws.on('message', (data) => {
    try {
      const msg = JSON.parse(data);
      if (msg.type === 'setName') {
        const oldName = clients.get(ws).name;
        clients.get(ws).name = msg.name;
        broadcast({ type: 'system', content: `${oldName} 改名为 ${msg.name}` });
      } else if (msg.type === 'chat') {
        broadcast({ type: 'chat', name: clients.get(ws).name, content: msg.content });
      }
    } catch (e) {
      console.error('消息解析错误:', e);
    }
  });

  ws.on('close', () => {
    broadcast({ type: 'system', content: `${clients.get(ws).name} 离开了聊天室` });
    console.log(`❌ ${clients.get(ws).name} 已断开`);
    clients.delete(ws);
  });
});

function broadcast(msg) {
  const data = JSON.stringify({ ...msg, time: new Date().toLocaleTimeString('zh-CN') });
  wss.clients.forEach((client) => {
    if (client.readyState === 1) client.send(data);
  });
}

server.listen(PORT, () => {
  console.log(`🚀 聊天服务器运行在 http://localhost:${PORT}`);
});
