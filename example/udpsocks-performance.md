# udpsocks performance and limit tests

Measured on 2026-10-11 against the working-tree implementation (base commit `3c00f04`).

## Results

With 1,200-byte payloads and sixteen concurrent clients, forward UDP delivered **14.60 MB/s (116.8 Mbps)** and reverse UDP delivered **15.03 MB/s (120.2 Mbps)**. Both measured zero loss. A single client had median round-trip latency of approximately **176–179 microseconds**.

At 8,192 bytes with sixteen clients, delivered throughput reached approximately **38 MB/s**, with **0.51–0.55% loss**. Direct UDP also lost packets in this case (**0.075%**). The first strict benchmark stopped on these timeouts; the final benchmark records timeouts as lost packets.

The limit tests passed at the configured boundaries, including the default 100-association and 100-destination limits. Overload testing confirmed that unrelated clients still respond and associations recover after the flood.

## Method

- Linux/ARM64, four logical CPUs, Go 1.27.0, `GOMAXPROCS=4`.
- Direct UDP, forward `udpsocks`, and reverse `R:udpsocks`, all over loopback. Both Chisel peers, the independent `txthinking/socks5` client, and the echo endpoint run in the benchmark process.
- Three runs per combination, one-second benchmark target per run; the table shows the median of each metric across those runs.
- One or sixteen independent clients, each with one request outstanding. Associations and destinations are warmed before timing.
- Default Chisel limits and socket buffers. This host reports default Linux receive/send buffers of 212,992 bytes; these were not increased for the measurements.
- A request counts as lost if its matching response does not arrive within 100 ms. Sequence numbers prevent late responses being counted against a later request.
- RTT percentiles sample successful responses; lost requests are excluded from those percentiles. Throughput includes the time spent waiting for losses.
- MB/s counts delivered application payload once per echo. It excludes SOCKS/SSH/WebSocket overhead and does not double-count the return direction.
- Allocation figures include both Chisel peers, the independent client, and the endpoint. Race instrumentation was disabled for performance measurements.

These are local measurements of this host and workload. A WAN adds network latency, transport congestion, and TCP head-of-line blocking; these measurements do not establish maximum WAN throughput.

## Echo benchmark results

| Path | Payload | Clients | Payload MB/s | Echoes/s | RTT p50 (us) | RTT p95 (us) | Loss (%) | B/attempt | Allocs/attempt |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| direct | 64 | 1 | 3.27 | 51101 | 12.2 | 21.2 | 0.0000 | 52 | 2 |
| direct | 64 | 16 | 7.26 | 113456 | 109.7 | 316.6 | 0.0000 | 52 | 2 |
| direct | 1200 | 1 | 88.91 | 74089 | 12.6 | 14.6 | 0.0000 | 52 | 2 |
| direct | 1200 | 16 | 129.59 | 107988 | 116.8 | 324.4 | 0.0000 | 52 | 2 |
| direct | 8192 | 1 | 475.06 | 57990 | 16.0 | 20.6 | 0.0000 | 52 | 2 |
| direct | 8192 | 16 | 636.57 | 77706 | 89.1 | 237.8 | 0.0750 | 52 | 1 |
| forward | 64 | 1 | 0.40 | 6235 | 158.0 | 258.6 | 0.0000 | 3414 | 53 |
| forward | 64 | 16 | 1.03 | 16044 | 853.0 | 2106.0 | 0.0000 | 3422 | 53 |
| forward | 1200 | 1 | 6.56 | 5469 | 175.7 | 308.2 | 0.0000 | 16792 | 61 |
| forward | 1200 | 16 | 14.60 | 12169 | 1099.0 | 2885.0 | 0.0000 | 16804 | 61 |
| forward | 8192 | 1 | 22.08 | 2695 | 325.5 | 730.6 | 0.0000 | 111462 | 95 |
| forward | 8192 | 16 | 38.29 | 4675 | 2442.0 | 5290.0 | 0.5495 | 111107 | 95 |
| reverse | 64 | 1 | 0.41 | 6421 | 142.2 | 261.1 | 0.0000 | 3414 | 53 |
| reverse | 64 | 16 | 1.01 | 15761 | 860.4 | 2147.0 | 0.0000 | 3423 | 53 |
| reverse | 1200 | 1 | 6.45 | 5372 | 178.9 | 299.4 | 0.0000 | 16792 | 61 |
| reverse | 1200 | 16 | 15.03 | 12521 | 1055.0 | 2799.0 | 0.0000 | 16805 | 61 |
| reverse | 8192 | 1 | 22.18 | 2707 | 328.3 | 699.0 | 0.0000 | 111462 | 95 |
| reverse | 8192 | 16 | 37.88 | 4624 | 2402.0 | 5127.0 | 0.5121 | 111102 | 95 |

The 1,200-byte tunnel paths allocate about 16.8 KB and 61 allocations per attempt in this whole-process benchmark. At 8,192 bytes, that rises to about 111 KB and 95 allocations. Allocation reduction is a useful candidate for subsequent profiling and optimization.

## Unpaced overload

Each path sends 20,000 datagrams of 1,200 bytes as quickly as its client can write them, with a concurrent response reader. Offered rates differ between paths. This measures burst loss and recovery rather than a sustainable constant-rate workload.

| Path | Offered packets/s | Received / 20,000 | Loss (%) | Successful RTT p50 (us) | Successful RTT p95 (us) |
| --- | ---: | ---: | ---: | ---: | ---: |
| direct | 193130 | 7218 | 63.91 | 549.7 | 2795.5 |
| forward | 95885 | 2274 | 88.63 | 9087.6 | 34495.8 |
| reverse | 105168 | 2077 | 89.61 | 10477.9 | 26154.9 |

All three paths passed 20 probes from an unrelated client during the workload and a recovery echo on the flooded connection afterward. No duplicated or corrupted received packets were detected. UDP socket buffers and the relay queue can both drop packets; this test does not attribute each loss to a specific queue.

## Limit coverage

| Limit or behavior | Verified |
| --- | --- |
| Associations per listener | Configured limit 2 and default 100, forward and reverse. The next association fails; existing UDP and CONNECT continue. Closing a control frees capacity. |
| Destinations per association | Configured limit 2 and default 100, forward and reverse. Over-limit requests never reach the new destination. Other associations have independent capacity. |
| Destination idle expiration | A 500 ms configured timeout frees capacity for a new destination without replacing its TCP control connection. |
| Datagram size | Exact 64-byte and 9,012-byte SOCKS packets succeed. One extra byte is dropped before reaching the endpoint. IPv6 header overhead is included. |
| Upper packet-size bound | Setting 70,000 clamps to 65,507. An exact 65,507-byte SOCKS datagram succeeds. The OS cannot send a larger IPv4 UDP datagram; oversized return packets are tested separately. |
| Oversized replies | Responses whose SOCKS header plus payload exceeds the limit are dropped; subsequent valid traffic still works. |
| Shared-reader overflow | A full 16-packet association queue drops overflow and does not block another association. Covered deterministically by `TestSocksUDPSharedRoutingAndOverflow`. |
| Invalid settings | Zero, negative, and malformed size/connection/deadline settings fall back to defaults; oversized size settings clamp. |
| Existing protocol limits | Malformed/unassociated packets and fragmentation are rejected; source-port conflicts and the one-pending-association-per-IP rule remain covered. |

The association and destination limits are per listener and per association respectively; they are not a process-wide socket or memory limit.

Final validation passed: `CHISEL_UDP_STRESS=1 GOMAXPROCS=4 go test -race ./...`, `go vet ./...`, and `git diff --check`.

## Reproduce

Run the benchmark without race instrumentation:

```sh
GOMAXPROCS=4 go test ./test/e2e -run "^$" -bench "^BenchmarkSocksUDP$" \
  -benchmem -benchtime=1s -count=3
```

Run the boundary and recovery tests:

```sh
go test -race ./test/e2e -run '^TestSocksUDP(AssociationLimit|DestinationLimit|PacketSizeLimit)$'
go test -race ./share/tunnel -run '^TestSocksUDP(LimitSettings|SharedRoutingAndOverflow|ExitLimitsAndRecovery)$'
```

Run the opt-in overload test:

```sh
CHISEL_UDP_STRESS=1 GOMAXPROCS=4 go test -v ./test/e2e -run "^TestSocksUDPOverload$"
```

Ordinary `go test ./...` runs the limit tests and skips the overload workload. Set `CHISEL_UDP_STRESS=1` to include it. Performance numbers above come from runs without the race detector; run the full suite with `-race` to check concurrency correctness.
