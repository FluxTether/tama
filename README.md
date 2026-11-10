# tama

TaskManager - Decentralized Project Task manager

## libp2p node

`main.go` starts a libp2p node and persists its identity between runs.

On first start an Ed25519 key pair is generated and stored in the data
directory. Later starts reuse that identity, so the peer ID stays stable.

### Build

```sh
go build -o tama .
```

Plain `go build` also works here and produces `./tama` because the package
directory is named `tama`.

### Run

```sh
go run .
```

By default all runtime files are written to `./data`, next to the code. Use
`-config` to point at another directory:

```sh
go run . -config ./data
```

Override the listen addresses with `-listen`:

```sh
go run . -listen "/ip4/0.0.0.0/tcp/4001,/ip4/0.0.0.0/udp/4001/quic-v1"
```

mDNS discovery is on by default; turn it off with `-mdns=false`.

## Configuration

Environment variables are loaded from `.env` in the working directory (a
missing file is fine); real environment variables take precedence. Start from
the committed template:

```sh
cp .env.example .env
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `WEB_PORT` | `8080` | HTTP port used by the web UI. |
| `WS_RELAY` | empty | Base WebSocket relay URL, e.g. `wss://ws.tridentlab.in`. Each peer joins a per-peer room. |
| `LIBP2P_RELAY` | empty | libp2p relay multiaddr **including the relay peer ID**. |

Known peers are listed in `data/peers.json` (a JSON array of peer ID strings)
and/or passed with the repeatable `-peer` flag:

```sh
go run . -peer 12D3KooW... -peer 12D3KooX...
```

```json
[
  "12D3KooW...",
  "12D3KooX..."
]
```

### Peer connectivity

tama connects only to **known peers**; it does not discover unknown ones.

- **LAN** — mDNS is enabled by default. It announces this node and reports
  peers found on the local network, but only known peers are dialed. Disable it
  with `-mdns=false`.
- **Internet lookup** — `WS_RELAY` is used as a per-peer message relay. Each
  node joins the room named after its own peer ID (`<WS_RELAY>/<peer-id>`) and
  answers signed queries. To locate a known peer, a node joins that peer's room,
  sends a signed `query`, and waits for a signed `response` containing the
  peer's current multiaddrs. No response means the peer is offline or not
  joined; the server itself reports no status.
- **Connectivity** — `LIBP2P_RELAY` is a libp2p circuit relay: the node dials it
  at startup and enables AutoRelay, so it can obtain a circuit-relay address and
  reach/be reached through the relay when behind NAT. DCUtR can then upgrade the
  connection to a direct hole-punched one.

The lookup flow is: **WS room query → signed address response → libp2p dial
(relay/hole-punch as needed)**. The WS relay only exchanges peer IDs and
addresses; task traffic stays on libp2p.

Every WS message is signed with the libp2p identity key and carries a nonce and
timestamp; receivers verify the signature against the sender's peer ID and
reject stale messages.

To host the traffic relay:

```sh
./tama -relay -listen "/ip4/0.0.0.0/tcp/4001,/ip4/0.0.0.0/udp/4001/quic-v1"
```

It prints addresses such as:

```
/ip4/203.0.113.10/tcp/4001/p2p/12D3KooW...
```

Use one of those as `LIBP2P_RELAY` on the other nodes. In relay mode the
`LIBP2P_RELAY` variable is ignored; the relay uses its own identity from its
data directory (keep it stable, since clients reference its peer ID).

## Files

| File | Contents |
| --- | --- |
| `data/identity.json` | Private key (base64, libp2p protobuf form), peer ID, creation time. Mode `0600`. |
| `data/peers.json` | JSON array of known peer ID strings. Optional. |

`data/identity.json` is secret: never commit it or share it. The `data/`
directory and `.env` are ignored by git, so database and UI state can live
there as well.

## Peer info

Peer info (peer ID, listen addresses, full dialable multiaddrs, protocols,
agent version) is kept **in RAM only** and rebuilt on every start. It is not
written to `data/` because listen ports, interfaces and NAT mappings change
between runs — a stored copy would quickly be wrong. The stable, secret part
that *must* persist is the identity key; the peer ID is derived from it.

At runtime peer info is used to:

- tell other peers how to dial this node (`<multiaddr>/p2p/<peer-id>`);
- advertise supported protocols and the agent version;
- display/share connection details in the UI or over an API;
- support reconnects, relays and the peerstore.

`node.Info(host)` returns the in-memory snapshot, and `(*PeerInfo).JSON()`
serializes it into a RAM buffer for an HTTP/WebSocket response.
