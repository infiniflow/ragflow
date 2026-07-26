# RAGFlow WhatsApp Gateway

This directory contains a minimal WhatsApp gateway built on top of
`@whiskeysockets/baileys`.

## Install

```bash
cd api/channels/whatsapp/gateway-node
npm install
```

## Run

```bash
WHATSAPP_GATEWAY_PORT=3005 \
WHATSAPP_GATEWAY_DATA_DIR=~/.ragflow/whatsapp-gateway \
npm start
```

## API

- `POST /whatsapp/:sessionKey/start`
- `GET /whatsapp/:sessionKey/status`
- `GET /whatsapp/:sessionKey/events/ws?after=<seq>` (WebSocket)
- `POST /whatsapp/:sessionKey/send`
- `POST /whatsapp/:sessionKey/stop`

## Notes

- Authentication state is persisted under `WHATSAPP_GATEWAY_DATA_DIR`.
- Scan the QR code exposed in `status.qr_data_url`.
- RAGFlow polls `status` and `events` and forwards inbound messages to the
  connected assistant.
