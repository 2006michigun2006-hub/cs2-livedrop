# cs2-livedrop

CS2 LiveDrop is a Go monolith (`chi`) that links CS2 Game State Integration events to real giveaway actions for viewers.

## What works now

- Steam auth + JWT
- Telegram bot chat invite flow (no Mini App required)
- Stream session start with invite link + QR generation
- Viewer join by invite + Steam login callback auto-join
- Event-driven giveaway rules (e.g. `ace`, `headshot`, `bomb_plant`)
- Dedicated invite-driven case simulator page (`/simulator.html`)
- Inventory rewards for giveaway winners (`skin` or `case`)
- Case opening flow (open won case -> random skin drop)
- Streamer event presets + custom editable rules (create/update/delete)
- Admin dashboard UX updated for step-by-step stream flow
- Wallet and lottery persistence in Postgres
- GSI packet idempotency (`sha256` de-dup)

## Quick start

```bash
docker compose up -d
cp .env.example .env
go run ./cmd/server
```

Open `http://localhost:8080`.

## Required env

- `DATABASE_URL`
- `JWT_SECRET`
- `BASE_URL` (public URL used for invite links and Steam callback)
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_BOT_USERNAME` (without `@`, for deep links)

## Main APIs

- Auth: `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/auth/steam/login`, `GET /api/auth/steam/callback`
- Streams (streamer/admin):
  - `POST /api/streams/start`
  - `GET /api/streams/me/active`
  - `POST /api/streams/{sessionID}/end`
  - `GET /api/streams/{sessionID}/participants`
  - `GET /api/streams/events/presets`
  - `POST /api/streams/{sessionID}/giveaways`
  - `GET /api/streams/{sessionID}/giveaways`
  - `PUT /api/streams/{sessionID}/giveaways/{ruleID}`
  - `DELETE /api/streams/{sessionID}/giveaways/{ruleID}`
- Inventory (authenticated viewer):
  - `GET /api/inventory/me`
  - `POST /api/inventory/open/{itemID}`
- Join flow:
  - `GET /invite/{inviteCode}`
  - `POST /api/streams/join/{inviteCode}` (already authenticated)
- Telegram bot webhook: `POST /api/telegram/webhook`
- GSI ingest: `POST /api/gsi`

## Telegram bot flow

1. Streamer starts session and optionally sends invite to chat.
2. Bot posts invite/deep link in chat.
3. Viewer opens invite link, logs in with Steam.
4. Steam callback auto-joins viewer to stream pool.
5. CS2 event arrives at `/api/gsi` and matching giveaway rules trigger weighted draw.
6. Winner receives reward in inventory; if reward type is `case`, viewer can open it in simulator.

## CS2 assets source

Frontend simulator pulls live case/skin names + icons from:

- `https://raw.githubusercontent.com/ByMykel/CSGO-API/main/public/api/en/crates.json`
- `https://raw.githubusercontent.com/ByMykel/CSGO-API/main/public/api/en/skins.json`

## Notes

- First registered account is auto-`admin`.
- Stream management is `streamer`/`admin` only.
- Set Telegram webhook to `https://<your-domain>/api/telegram/webhook`.
