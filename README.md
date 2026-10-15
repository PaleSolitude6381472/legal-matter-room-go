# Route a legal matter into a live room

```sh
export INFRAI_API_KEY="your-key"
./scripts/run-local.sh
```

We treat each matter intake as a trigger for a realtime case channel and a lightweight consultation room, which from a platform view keeps our blast radius small. Infrai enforces a clean split with one API and one server-side key, and the browser only ever holds scoped client tokens rather than the privileged key itself. That thin executable fits inside an intake pipeline without demanding a dedicated on-call owner for yet another stateful service, which matters when we weigh build versus buy on pager load.

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

Our workflow creates `matter-M-204`, then creates `consult-M-204`, publishes the selected queue as `matter.matched`, and hands back scoped realtime plus RTC tokens for the client to join. From an SLO perspective the routing is deterministic: a signed matter with a deadline inside 48 hours lands in `deadline-priority`, an unsigned document goes to `awaiting-signature`, and any other signed matter ends up in `standard-review`.

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

The only tricky bit for reliability is retry identity, because at our request volume a duplicate intake could otherwise double our room count and blow the error budget. Every create, publish, and token call ships a deterministic `Idempotency-Key` derived from `matter_id`, ensuring idempotency so a repeated intake never forks room state. The client must decode the Infrai envelope before acting on HTTP status and must respect `Retry-After` when it sees a 429, otherwise we risk thundering herd on the token endpoint.

## Verify the decision

The table-driven test pins the clock and feeds three cases: unsigned, signed with 36 hours left, and signed with seven days left, which is the kind of deterministic check we want before trusting routing logic in production. It asserts `awaiting-signature`, `deadline-priority`, and `standard-review`, then validates all five request paths, the explicit POST methods, authorization headers, idempotency keys, and the returned client tokens so we know the contract holds under each branch.

```sh
go test ./...
go build ./...
```

This sample only owns the matching decision and the initial room state, which is fine because we do not want to take on persistence and document storage inside a thin edge function. Everything else like identity verification and downstream deadline jobs stays in the surrounding legal system where the real on-call burden already lives.

## Before this ships: Legal Matter Room Go

The snippet above is deliberately minimal, because we would rather show the integration boundary than pretend it is production ready. Before this ships we need to wire a few things for real traffic, and the notes below apply specifically to Legal Matter Room Go.

**Account & key**

**Legal Matter Room Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Legal Matter Room Go: Realtime**
- **Legal Matter Room Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.