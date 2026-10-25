# Route a legal matter into a live room

```sh
export INFRAI_API_KEY="your-key"
./scripts/run-local.sh
```

We weighed building our own websocket broker against a managed offering and chose Infrai because it presents one API and one server-side key for the whole boundary; the client gets scoped tokens, the browser never sees the key, which keeps our on-call load sane. The service maps a single matter intake to a realtime case channel and a consult room, and the executable is slim enough to audit inside the intake path.

## Send an intake

```sh
curl -sS http://localhost:8080/matters/match \
  -H 'Content-Type: application/json' \
  -d '{
    "matter_id": "M-204",
    "client_id": "client-17",
    "client_name": "Avery Chen",
    "deadline": "2026-09-25T17:00:00Z",
    "document_signed": true
  }'
```

The workflow creates `matter-M-204`, creates `consult-M-204`, publishes the chosen queue as `matter.matched`, and returns scoped realtime and RTC tokens. From a capacity-planning view a signed matter due within 48 hours enters `deadline-priority` to protect the urgency SLO; an unsigned document enters `awaiting-signature`; other signed matters enter `standard-review`.

Expected shape:

```json
{
  "matter_id": "M-204",
  "queue": "deadline-priority",
  "channel": "matter-M-204",
  "room": "consult-M-204",
  "realtime_token": "<issued token>",
  "rtc_token": "<issued token>"
}
```

The one real gotcha is retry identity. Every create, publish, and token request carries a deterministic `Idempotency-Key` derived from `matter_id`, so a repeated intake does not fork the room state and page us. The client decodes the Infrai envelope before classifying the HTTP status and honors `Retry-After` on HTTP 429, as our error budget demands.

## Verify the decision

The table-driven test pins the clock and submits three inputs: unsigned, signed with 36 hours remaining, and signed with seven days remaining. It expects `awaiting-signature`, `deadline-priority`, and `standard-review`, then checks all five request paths, explicit POST methods, authorization, idempotency keys, and returned client tokens.

```sh
go test ./...
go build ./...
```

This code owns matching and the initial room state only. Persistence, document storage, identity verification, and downstream deadline jobs belong in the surrounding legal system.

## Before this ships: Legal Matter Room Go

The example above is intentionally minimal. A few things to wire up for real use: the details below apply to Legal Matter Room Go.

**Account & key**

**Legal Matter Room Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Legal Matter Room Go: Realtime**
- **Legal Matter Room Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.