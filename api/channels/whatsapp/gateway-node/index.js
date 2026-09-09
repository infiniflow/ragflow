import http from 'node:http';
import crypto from 'node:crypto';
import os from 'node:os';
import path from 'node:path';
import { mkdir } from 'node:fs/promises';

import makeWASocket, {
  Browsers,
  DisconnectReason,
  fetchLatestBaileysVersion,
  useMultiFileAuthState,
} from 'baileys';
import QRCode from 'qrcode';

const PORT = Number.parseInt(process.env.WHATSAPP_GATEWAY_PORT || '3005', 10);
const HOST = process.env.WHATSAPP_GATEWAY_HOST || '127.0.0.1';
const AUTH_TOKEN = String(process.env.WHATSAPP_GATEWAY_TOKEN || '').trim();
const WS_MAGIC = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11';
const DATA_DIR =
  process.env.WHATSAPP_GATEWAY_DATA_DIR ||
  path.join(os.homedir(), '.ragflow', 'whatsapp-gateway');

function now() {
  return Date.now() / 1000;
}

function normalizeJid(chatId) {
  const raw = String(chatId || '').trim();
  if (!raw) {
    return '';
  }
  if (raw.includes('@')) {
    return raw;
  }
  const digits = raw.replace(/\D/g, '');
  if (!digits) {
    return '';
  }
  return `${digits}@s.whatsapp.net`;
}

function detectChatType(jid) {
  if (jid.endsWith('@g.us')) {
    return 'group';
  }
  if (jid.endsWith('@newsletter')) {
    return 'channel';
  }
  return 'dm';
}

function extractText(message) {
  if (!message) {
    return '';
  }
  return (
    message.conversation ||
    message.extendedTextMessage?.text ||
    message.imageMessage?.caption ||
    message.videoMessage?.caption ||
    message.documentMessage?.caption ||
    message.buttonsResponseMessage?.selectedButtonId ||
    message.listResponseMessage?.title ||
    ''
  ).trim();
}

function safeMessageKey(message) {
  return {
    remoteJid: message.key?.remoteJid || '',
    fromMe: Boolean(message.key?.fromMe),
    id: message.key?.id || '',
    participant: message.key?.participant || '',
  };
}

function buildWsFrame(text) {
  const data = Buffer.from(String(text), 'utf8');
  let header;
  if (data.length < 126) {
    header = Buffer.alloc(2);
    header[1] = data.length;
  } else if (data.length < 65536) {
    header = Buffer.alloc(4);
    header[1] = 126;
    header.writeUInt16BE(data.length, 2);
  } else {
    header = Buffer.alloc(10);
    header[1] = 127;
    header.writeBigUInt64BE(BigInt(data.length), 2);
  }
  header[0] = 0x81;
  return Buffer.concat([header, data]);
}

function sendWsText(socket, payload) {
  socket.write(buildWsFrame(JSON.stringify(payload)));
}

function isAuthorized(req) {
  if (!AUTH_TOKEN) {
    return true;
  }
  const auth = String(req.headers.authorization || '').trim();
  return auth === `Bearer ${AUTH_TOKEN}`;
}

class WhatsAppSession {
  constructor(sessionKey) {
    this.sessionKey = sessionKey;
    this.sessionDir = path.join(DATA_DIR, sessionKey);
    this.status = 'stopped';
    this.lastError = '';
    this.qrDataUrl = '';
    this.qrUpdatedAt = 0;
    this.connectedAt = 0;
    this.sessionId = '';
    this.authRegistered = false;
    this.lastSnapshotAt = 0;
    this.eventSeq = 0;
    this.events = [];
    this.messageStore = new Map();
    this.sock = null;
    this.saveCreds = null;
    this.starting = null;
    this.stopping = false;
    this.restartTimer = null;
    this.subscribers = new Set();
  }

  addSubscriber(socket, afterSeq) {
    const subscriber = { socket };
    this.subscribers.add(subscriber);
    socket.on('close', () => {
      this.subscribers.delete(subscriber);
    });
    socket.on('error', () => {
      this.subscribers.delete(subscriber);
    });

    sendWsText(socket, { type: 'snapshot', data: this.snapshot() });
    const backlog = this.listEvents(afterSeq);
    for (const event of backlog.items) {
      sendWsText(socket, { type: 'event', data: event });
    }
  }

  broadcast(message) {
    for (const subscriber of this.subscribers) {
      try {
        sendWsText(subscriber.socket, message);
      } catch {
        this.subscribers.delete(subscriber);
      }
    }
  }

  snapshot() {
    return {
      session_key: this.sessionKey,
      status: this.status,
      last_error: this.lastError || null,
      qr_data_url: this.qrDataUrl || null,
      qr_updated_at: this.qrUpdatedAt || null,
      connected_at: this.connectedAt || null,
      session_id: this.sessionId || null,
      auth_registered: this.authRegistered,
      last_snapshot_at: this.lastSnapshotAt || null,
      event_cursor: this.eventSeq,
      event_queue_size: this.events.length,
    };
  }

  listEvents(afterSeq) {
    const after = Number.isFinite(afterSeq) ? afterSeq : 0;
    return {
      next_cursor: this.eventSeq,
      items: this.events.filter((event) => event.seq > after),
    };
  }

  async start() {
    if (this.starting) {
      return this.starting;
    }
    if (this.sock) {
      return;
    }
    this.stopping = false;
    this.starting = this._start().finally(() => {
      this.starting = null;
    });
    return this.starting;
  }

  async _start() {
    await mkdir(this.sessionDir, { recursive: true });
    this.status = 'connecting';
    this.lastError = '';
    const { state, saveCreds } = await useMultiFileAuthState(this.sessionDir);
    this.saveCreds = saveCreds;
    this.authRegistered = Boolean(state?.creds?.registered);
    if (!this.authRegistered) {
      this.status = 'qr';
    }
    const { version } = await fetchLatestBaileysVersion();
    const sock = makeWASocket({
      auth: state,
      version,
      browser: Browsers.ubuntu('RAGFlow'),
      printQRInTerminal: false,
      markOnlineOnConnect: false,
      syncFullHistory: false,
      getMessage: async (key) => {
        if (!key?.id) {
          return undefined;
        }
        return this.messageStore.get(key.id);
      },
    });
    this.sock = sock;

    sock.ev.on('creds.update', this.saveCreds);
    sock.ev.on('connection.update', (update) => {
      void this._handleConnectionUpdate(update);
    });
    sock.ev.on('messages.upsert', (update) => {
      void this._handleMessagesUpsert(update);
    });

    this.lastSnapshotAt = now();
  }

  async _handleConnectionUpdate(update) {
    if (update.qr) {
      this.status = 'qr';
      this.lastError = '';
      this.qrUpdatedAt = now();
      this.qrDataUrl = await QRCode.toDataURL(update.qr, {
        errorCorrectionLevel: 'M',
        margin: 2,
        scale: 8,
      });
      this.lastSnapshotAt = now();
      this.broadcast({ type: 'snapshot', data: this.snapshot() });
    }

    if (update.connection === 'open') {
      this.status = 'connected';
      this.lastError = '';
      this.connectedAt = now();
      this.qrDataUrl = '';
      this.sessionId = this.sock?.user?.id || this.sessionId;
      this.authRegistered = true;
      this.lastSnapshotAt = now();
      this.broadcast({ type: 'snapshot', data: this.snapshot() });
      return;
    }

    if (update.connection === 'close') {
      const reason = update.lastDisconnect?.error?.output?.statusCode;
      const loggedOut = reason === DisconnectReason.loggedOut;
      this.status = loggedOut ? 'error' : 'disconnected';
      this.lastError =
        update.lastDisconnect?.error?.message ||
        (loggedOut ? 'WhatsApp session logged out.' : 'WhatsApp session disconnected.');
      this.lastSnapshotAt = now();
      this.sock = null;
      this.saveCreds = null;
      this.authRegistered = false;
      this.broadcast({ type: 'snapshot', data: this.snapshot() });

      if (!this.stopping && !loggedOut) {
        clearTimeout(this.restartTimer);
        this.restartTimer = setTimeout(() => {
          void this.start();
        }, 3000);
      }
    }
  }

  async _handleMessagesUpsert(update) {
    for (const message of update.messages || []) {
      const key = safeMessageKey(message);
      if (key.fromMe || !key.id || !key.remoteJid) {
        continue;
      }
      const text = extractText(message.message);
      if (!text) {
        continue;
      }
      const jid = key.remoteJid;
      const event = {
        seq: ++this.eventSeq,
        kind: 'message',
        message_id: key.id,
        chat_id: jid,
        chat_type: detectChatType(jid),
        sender_id: key.participant || jid,
        text,
        raw: {
          key,
          message: message.message,
          pushName: message.pushName || '',
          messageTimestamp: message.messageTimestamp || 0,
        },
      };
      this.events.push(event);
      this.messageStore.set(key.id, message);
      this.broadcast({ type: 'event', data: event });
      if (this.events.length > 1000) {
        const dropped = this.events.slice(0, this.events.length - 500);
        this.events = this.events.slice(-500);
        for (const oldEvent of dropped) {
          if (oldEvent.kind === 'message' && oldEvent.message_id) {
            this.messageStore.delete(oldEvent.message_id);
          }
        }
      }
      this.lastSnapshotAt = now();
    }
  }

  async send(payload) {
    if (!this.sock) {
      throw new Error('WhatsApp session is not running.');
    }
    const jid = normalizeJid(payload.chat_id);
    if (!jid) {
      throw new Error(`Invalid chat_id: ${payload.chat_id}`);
    }
    const text = String(payload.text || '');
    const options = {};
    if (payload.reply_to_message_id) {
      const quoted = this.messageStore.get(String(payload.reply_to_message_id));
      if (quoted) {
        options.quoted = quoted;
      }
    }
    await this.sock.sendMessage(jid, { text }, options);
    this.lastSnapshotAt = now();
  }

  async stop() {
    this.stopping = true;
    clearTimeout(this.restartTimer);
    this.restartTimer = null;
    const sock = this.sock;
    this.sock = null;
    this.saveCreds = null;
    if (sock) {
      try {
        sock.end?.(undefined);
      } catch {
        try {
          sock.ws?.close?.();
        } catch {
          // ignore
        }
      }
    }
    this.status = 'stopped';
    this.lastError = '';
    this.qrDataUrl = '';
    this.qrUpdatedAt = 0;
    this.connectedAt = 0;
    this.sessionId = '';
    this.lastSnapshotAt = now();
    this.subscribers.clear();
    this.events = [];
    this.messageStore.clear();
  }
}

const sessions = new Map();

function getSession(sessionKey) {
  const key = String(sessionKey || '').trim() || 'default';
  let session = sessions.get(key);
  if (!session) {
    session = new WhatsAppSession(key);
    sessions.set(key, session);
  }
  return session;
}

function getExistingSession(sessionKey) {
  const key = String(sessionKey || '').trim() || 'default';
  return sessions.get(key) || null;
}

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) {
    chunks.push(chunk);
  }
  if (!chunks.length) {
    return {};
  }
  const raw = Buffer.concat(chunks).toString('utf8');
  if (!raw.trim()) {
    return {};
  }
  return JSON.parse(raw);
}

function sendJson(res, statusCode, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(statusCode, {
    'content-type': 'application/json; charset=utf-8',
    'content-length': Buffer.byteLength(body),
  });
  res.end(body);
}

function sendError(res, statusCode, message) {
  sendJson(res, statusCode, { code: statusCode, message, data: null });
}

const server = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url || '/', `http://${req.headers.host || 'localhost'}`);
    const parts = url.pathname.split('/').filter(Boolean);

    if (req.method === 'GET' && url.pathname === '/health') {
      return sendJson(res, 200, { ok: true });
    }

    if (parts[0] !== 'whatsapp' || parts.length < 2) {
      return sendError(res, 404, 'not found');
    }

    if (!isAuthorized(req)) {
      return sendError(res, 401, 'unauthorized');
    }

    const sessionKey = decodeURIComponent(parts[1]);

    if (req.method === 'POST' && parts.length === 3 && parts[2] === 'start') {
      const session = getSession(sessionKey);
      await session.start();
      return sendJson(res, 200, { code: 0, message: '', data: session.snapshot() });
    }

    if (req.method === 'GET' && parts.length === 3 && parts[2] === 'status') {
      const session = getExistingSession(sessionKey);
      if (!session) {
        return sendError(res, 404, 'session not found');
      }
      return sendJson(res, 200, { code: 0, message: '', data: session.snapshot() });
    }

    if (req.method === 'POST' && parts.length === 3 && parts[2] === 'send') {
      const session = getExistingSession(sessionKey);
      if (!session) {
        return sendError(res, 404, 'session not found');
      }
      const body = await readBody(req);
      await session.send(body);
      return sendJson(res, 200, { code: 0, message: '', data: true });
    }

    if (req.method === 'POST' && parts.length === 3 && parts[2] === 'stop') {
      const session = getExistingSession(sessionKey);
      if (!session) {
        return sendError(res, 404, 'session not found');
      }
      await session.stop();
      sessions.delete(sessionKey);
      return sendJson(res, 200, { code: 0, message: '', data: true });
    }

    return sendError(res, 404, 'not found');
  } catch (error) {
    // Log the full error server-side for debugging, but return a
    // generic message to the client — error.message can leak
    // filesystem paths, internal hostnames, library internals, etc.
    console.error(error);
    return sendJson(res, 500, { code: 500, message: 'internal error', data: null });
  }
});

server.on('upgrade', (req, socket) => {
  try {
    const url = new URL(req.url || '/', `http://${req.headers.host || 'localhost'}`);
    const parts = url.pathname.split('/').filter(Boolean);
    if (parts[0] !== 'whatsapp' || parts.length !== 4 || parts[2] !== 'events' || parts[3] !== 'ws') {
      socket.destroy();
      return;
    }
    if (!isAuthorized(req)) {
      socket.destroy();
      return;
    }
    const sessionKey = decodeURIComponent(parts[1]);
    const key = req.headers['sec-websocket-key'];
    if (!key) {
      socket.destroy();
      return;
    }
    const accept = crypto.createHash('sha1').update(`${key}${WS_MAGIC}`).digest('base64');
    socket.write(
      [
        'HTTP/1.1 101 Switching Protocols',
        'Upgrade: websocket',
        'Connection: Upgrade',
        `Sec-WebSocket-Accept: ${accept}`,
        '',
        '',
      ].join('\r\n'),
    );
    socket.setNoDelay(true);
    const session = getExistingSession(sessionKey);
    if (!session) {
      socket.destroy();
      return;
    }
    const after = Number.parseInt(url.searchParams.get('after') || '0', 10) || 0;
    session.addSubscriber(socket, after);
    socket.on('end', () => {
      session.subscribers.forEach((subscriber) => {
        if (subscriber.socket === socket) {
          session.subscribers.delete(subscriber);
        }
      });
    });
  } catch (error) {
    console.error(error);
    socket.destroy();
  }
});

server.listen(PORT, HOST, () => {
  console.log(`WhatsApp gateway listening on http://${HOST}:${PORT}`);
});
