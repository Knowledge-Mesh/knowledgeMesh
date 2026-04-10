# Debug: Relay Stream Failure — `NewStream: context deadline exceeded`

## Scenario

- **Buyer**: Machine A behind NAT on mobile network
- **Seller**: Machine B behind NAT on mobile network
- **Relay**: AWS instance with public IP, open ports 4001, 8090, 9091, 443. Circuit relay v2 (`internal/relay`).
- Both machines connect to the relay successfully.
- Connection between buyer and seller is established (via relay).
- `h.NewStream(ctx, target, pid)` fails with **`context deadline exceeded`**.
- Works when: no relay needed, both machines have public IPs, or both are on the same subnet.

### Error Logs

**Seller:**
```
all retries for hole punch with peer 12D3KooWRdYaB4abQ25bxE4qZnoMZbdwE5LBnS1EspkGcC3nZzNq failed
```

**Buyer:**
```
[holepunch] upgrade failure peer=12D3KooWDvctqkPwQzeoGXNbSJZLRfMzqNbGD1ot8EUF5NPHHxry:
  failed to initiateHolePunch: failed to read CONNECT message from remote peer:
  stream reset (remote): code: 0x0: transport error: stream reset by remote, error code: 0
```

A minimal ping-pong test through the same relay also reproduces the failure.

---

## Root Cause Analysis

### 1. NOT A BUG (corrected): `res.MaxCircuits` is per-peer in this libp2p version

**File:** `internal/relay/serve.go:93`

```go
res.MaxCircuits = cfg.MaxCircuitsPeer       // 4 — per-peer limit (correct)
```

In libp2p v0.44.0, `relayv2.Resources.MaxCircuits` is documented as "maximum number of open relay connections **for each peer**" (per-peer, not global). The assignment is correct. There is no separate `MaxCircuitsPerPeer` field in this version. Default is 16; setting to 4 is fine.

---

### 2. CRITICAL: Relay circuit duration is only 120 seconds

**File:** `internal/relay/serve.go:36, 94-96`

```go
defaultRelayCircuitDurationS = 120   // only 2 minutes

res.Limit = &relayv2.RelayLimit{
    Duration: cfg.CircuitDuration,    // 120s
    Data:     cfg.MaxBandwidthPeer,   // 4 MiB
}
```

Each relayed connection (circuit) is killed by the relay after 120 seconds. The failure sequence:

1. Seller starts, connects to relay, AutoRelay creates a relay reservation.
2. Buyer connects to seller through the relay circuit.
3. The circuit's 120s timer starts ticking.
4. `HolePunchManager` sees relay-only connection, starts DCUtR attempts every 10-20s.
5. DCUtR fails (both behind mobile NAT) — consumes time and circuit data budget.
6. By the time the application's `NewStream` call is attempted, the circuit has either **expired** (120s elapsed) or had its **4 MiB data budget consumed** by DCUtR protocol traffic.
7. The relay kills the circuit, but QUIC doesn't immediately detect the dead path (idle timeout ~30s).
8. `ConnectToPeer` sees `Connectedness == Connected` (stale state) and skips redialing.
9. `NewStream` tries protocol negotiation on the dead circuit.
10. **Result:** `context deadline exceeded`.

---

### 3. HIGH: Seller posts addresses before relay reservation completes

**File:** `internal/seller/serve.go:94-101`

```go
// Called immediately after host creation — AutoRelay is async!
listenAddrs := make([]string, 0, len(h.Addrs()))
for _, a := range h.Addrs() {
    listenAddrs = append(listenAddrs, a.String())
}
if _, err := cc.PostSellerPresence(tok, pid, listenAddrs); err != nil {
    log.Printf("warning: post presence to control: %v", err)
}
```

`h.Addrs()` is called immediately after host creation. AutoRelay runs asynchronously — the relay reservation (which produces the `/p2p-circuit` addresses) may not have completed yet. The control plane may receive **only local/LAN addresses** that are unreachable from the buyer's mobile network.

The buyer gets these addresses from the control plane match response and cannot dial the seller.

---

### 4. MEDIUM: `EnableNATService()` on `ForceReachabilityPrivate` nodes

**File:** `internal/network/hostconfig.go:277-278`

```go
libp2p.EnableNATService(),         // offers dial-back probes for other peers
libp2p.ForceReachabilityPrivate(), // but this node IS behind NAT
```

These are contradictory. `EnableNATService()` makes the peer offer AutoNAT dial-back probes for other peers. `ForceReachabilityPrivate()` means this peer is behind NAT and **cannot actually perform dial-back**. This wastes resources and may interfere with the relay's own AutoNAT probing.

---

### 5. HIGH: No connection liveness check before `NewStream`

**File:** `internal/network/network.go:148-153`

```go
func ConnectToPeer(ctx context.Context, h host.Host, id peer.ID, transportAddrs []string) error {
    // Reuse an existing session when already connected instead of redialing per request.
    if h.Network().Connectedness(id) == network.Connected {
        return nil   // trusts connection is alive — no health check
    }
    // ...
}
```

After the relay kills a circuit (120s expiry), there is a window (up to QUIC idle timeout ~30s) where `Connectedness` still reports `Connected`. `ConnectToPeer` skips redialing, and the subsequent `NewStream` hits the dead circuit.

---

### 6. MEDIUM: Single relay, no redundancy

**File:** `internal/network/hostconfig.go:286-289`

```go
libp2p.EnableAutoRelayWithStaticRelays(relayInfos,
    autorelay.WithBootDelay(0),
    autorelay.WithMinCandidates(1),
    autorelay.WithNumRelays(1),
    autorelay.WithMaxCandidates(1),   // single point of failure
    autorelay.WithBackoff(5*time.Second)),
```

Only one relay candidate/slot is configured. If the relay connection degrades or the circuit expires, there is no fallback relay.

---

## Summary

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 1 | ~~`MaxCircuits` global vs per-peer~~ (not a bug — per-peer in v0.44) | `relay/serve.go:93` | ~~Critical~~ N/A |
| 2 | Circuit duration 120s + data limit 4 MiB — circuits die before/during stream use | `relay/serve.go:36` | **Critical** |
| 3 | Seller posts addresses before relay addrs available | `seller/serve.go:94-101` | **High** |
| 4 | `EnableNATService` on `ForceReachabilityPrivate` nodes | `hostconfig.go:277-278` | **Medium** |
| 5 | No connection liveness check before `NewStream` | `network.go:150-152` | **High** |
| 6 | Single relay, no fallback | `hostconfig.go:286-289` | **Medium** |

Issues **2** and **3** together are the most likely root cause. The relay circuit only lives 120 seconds with a 4 MiB data budget. DCUtR hole punch attempts consume time and data on the circuit. Meanwhile, the seller may not post relay addresses in time, leaving the buyer with unreachable local-only addresses.

---

## Recommended Fixes

### Issue 1 — No change needed

`MaxCircuits` is per-peer in libp2p v0.44.0. The original code is correct.

### Issue 2 — Increase circuit duration and data limit

```go
// Before:
defaultRelayCircuitDurationS = 120   // 2 minutes

// After:
defaultRelayCircuitDurationS = 3600  // 1 hour
```

Also consider increasing the data limit from 4 MiB to something larger (e.g. 32 MiB) for inference payloads.

### Issue 3 — Wait for relay addresses before posting presence

Add a delay or poll `h.Addrs()` until relay circuit addresses appear before calling `PostSellerPresence`. For example:

```go
// Wait up to 15s for relay addresses to appear
deadline := time.After(15 * time.Second)
tick := time.NewTicker(500 * time.Millisecond)
defer tick.Stop()
for {
    addrs := h.Addrs()
    for _, a := range addrs {
        if _, err := a.ValueForProtocol(ma.P_CIRCUIT); err == nil {
            goto ready // found a relay address
        }
    }
    select {
    case <-deadline:
        log.Printf("warning: no relay addresses after 15s, posting local-only presence")
        goto ready
    case <-tick.C:
    }
}
ready:
```

### Issue 4 — Remove `EnableNATService` from private peers

```go
// Remove this line from hostconfig.go for buyer/seller hosts:
libp2p.EnableNATService(),
```

Keep it only on the relay server (which has `ForceReachabilityPublic`).

### Issue 5 — Add connection health check

Before trusting an existing connection, verify the stream can actually be opened. Alternatively, force a reconnect when `NewStream` fails:

```go
func ConnectToPeer(ctx context.Context, h host.Host, id peer.ID, transportAddrs []string) error {
    if h.Network().Connectedness(id) == network.Connected {
        // Verify the connection is alive by checking for recent activity
        // or just return nil and let the caller retry on NewStream failure.
        return nil
    }
    // ...
}
```

A wrapper around inference sending could retry with a forced reconnect:

```go
resp, err := sendRequest(ctx, h, target, pid, body)
if err != nil {
    // Close stale connections and force redial
    for _, c := range h.Network().ConnsToPeer(target) {
        _ = c.Close()
    }
    _ = h.Connect(ctx, peer.AddrInfo{ID: target, Addrs: addrs})
    resp, err = sendRequest(ctx, h, target, pid, body)
}
```

### Issue 6 — Increase relay redundancy

```go
autorelay.WithNumRelays(2),
autorelay.WithMaxCandidates(3),
autorelay.WithMinCandidates(2),
```

---

*Date: 2026-04-10*
*Branch: debug/relay-stream-failure*
*Based on: commit 1cdb032 (system/p2p_for_nat_network)*
