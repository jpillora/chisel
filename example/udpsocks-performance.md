# udpsocks performance and limit tests

Measured on 2026-10-11 against the UDP reply-source fix in the working tree (base commit `e7a9879`). Each association now uses unconnected outgoing sockets shared across destinations, with at most one socket per address family.

## Results

With 1,200-byte payloads and sixteen concurrent clients, forward UDP delivered **17.17 MB/s (137.4 Mbps)** and reverse UDP delivered **16.88 MB/s (135.0 Mbps)**. Both measured zero loss. A single client had median round-trip latency of approximately **176–192 microseconds**.

At 8,192 bytes with sixteen clients, delivered throughput reached **40.74 MB/s forward and 43.29 MB/s reverse**, with **0.43–0.50% loss**. Direct UDP also lost packets in this case (**0.059%**). The benchmark records timeouts as lost packets.

The limit tests passed at the configured boundaries, including the default 100-association and 100-destination limits. Overload testing confirmed that unrelated clients still respond and associations recover after the flood.

## Method

- Linux/ARM64, four logical CPUs, Go 1.27.0, `GOMAXPROCS=4`.
- Direct UDP, forward `udpsocks`, and reverse `R:udpsocks`, all over loopback. Both Chisel peers, the independent `txthinking/socks5` client, and the echo endpoint run in the benchmark process.
- Three runs per combination, one-second benchmark target per run; the table shows the median of each metric across those runs.
- Payloads of 1,200 and 8,192 bytes. One or sixteen independent clients, each with one request outstanding. Associations and destinations are warmed before timing.
- Default Chisel limits and socket buffers. This host reports default Linux receive/send buffers of 212,992 bytes; these were not increased for the measurements.
- A request counts as lost if its matching response does not arrive within 100 ms. Sequence numbers prevent late responses being counted against a later request.
- RTT percentiles sample successful responses; lost requests are excluded from those percentiles. Throughput includes the time spent waiting for losses.
- MB/s counts delivered application payload once per echo. It excludes SOCKS/SSH/WebSocket overhead and does not double-count the return direction.
- Allocation figures include both Chisel peers, the independent client, and the endpoint. Race instrumentation was disabled for performance measurements.

These are local measurements of this host and workload. A WAN adds network latency, transport congestion, and TCP head-of-line blocking; these measurements do not establish maximum WAN throughput.

## Echo benchmark results

| Path | Payload | Clients | Payload MB/s | Echoes/s | RTT p50 (us) | RTT p95 (us) | Loss (%) | B/attempt | Allocs/attempt |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| direct | 1200 | 1 | 88.87 | 74056 | 12.6 | 14.6 | 0.0000 | 52 | 2 |
| direct | 1200 | 16 | 137.76 | 114797 | 112.8 | 304.8 | 0.0000 | 52 | 2 |
| direct | 8192 | 1 | 499.26 | 60945 | 15.5 | 19.1 | 0.0000 | 52 | 2 |
| direct | 8192 | 16 | 712.51 | 86976 | 82.4 | 205.7 | 0.0589 | 52 | 1 |
| forward | 1200 | 1 | 6.27 | 5225 | 175.9 | 319.6 | 0.0000 | 16827 | 65 |
| forward | 1200 | 16 | 17.17 | 14310 | 924.6 | 2464.0 | 0.0000 | 16839 | 65 |
| forward | 8192 | 1 | 21.32 | 2602 | 327.4 | 729.8 | 0.0000 | 111496 | 99 |
| forward | 8192 | 16 | 40.74 | 4973 | 2195.0 | 5087.0 | 0.4958 | 111139 | 99 |
| reverse | 1200 | 1 | 6.15 | 5123 | 191.7 | 309.1 | 0.0000 | 16827 | 65 |
| reverse | 1200 | 16 | 16.88 | 14069 | 920.4 | 2458.0 | 0.0000 | 16840 | 65 |
| reverse | 8192 | 1 | 22.36 | 2729 | 325.2 | 648.0 | 0.0000 | 111495 | 99 |
| reverse | 8192 | 16 | 43.29 | 5285 | 2083.0 | 4716.0 | 0.4348 | 111201 | 99 |

The 1,200-byte tunnel paths allocate about 16.8 KB and 65 allocations per attempt in this whole-process benchmark. At 8,192 bytes, that rises to about 111 KB and 99 allocations. Allocation reduction is a useful candidate for subsequent profiling and optimization.

## Unpaced overload

Each path sends 20,000 datagrams of 1,200 bytes as quickly as its client can write them, with a concurrent response reader. Offered rates differ between paths. This measures burst loss and recovery rather than a sustainable constant-rate workload.

| Path | Offered packets/s | Received / 20,000 | Loss (%) | Successful RTT p50 (us) | Successful RTT p95 (us) |
| --- | ---: | ---: | ---: | ---: | ---: |
| direct | 165172 | 3455 | 82.72 | 1630.2 | 10664.1 |
| forward | 88916 | 2184 | 89.08 | 9035.8 | 17683.1 |
| reverse | 98778 | 2103 | 89.48 | 9294.8 | 26451.9 |

All three paths passed 20 probes from an unrelated client during the workload and a recovery echo on the flooded connection afterward. No duplicated or corrupted received packets were detected. UDP socket buffers and the relay queue can both drop packets; this test does not attribute each loss to a specific queue.

## Limit coverage

| Limit or behavior | Verified |
| --- | --- |
| Associations per listener | Configured limit 2 and default 100, forward and reverse. The next association fails; existing UDP and CONNECT continue. Closing a control frees capacity. |
| Destinations per association | Configured limit 2 and default 100, forward and reverse. Over-limit requests never reach the new destination. Other associations have independent capacity. |
| Destination idle expiration | A 500 ms configured timeout frees capacity for a new destination without replacing its TCP control connection. Expiration keeps outgoing sockets and their source ports intact. |
| Changed reply endpoint | Two-block TFTP exchanges switch server ports and preserve the client transfer ID, forward/reverse over IPv4/IPv6. A different reply source IP is also covered when the OS permits binding an alternate loopback address; that variant skips when the address is unavailable. Two simultaneous associations use separate exit sockets. |
| Datagram size | Exact 64-byte and 9,012-byte SOCKS packets succeed. One extra byte is dropped before reaching the endpoint. IPv6 header overhead is included. |
| Upper packet-size bound | Setting 70,000 clamps to 65,507. An exact 65,507-byte SOCKS datagram succeeds on Linux. This boundary case skips if an independent native UDP send reports a smaller OS limit, as macOS does by default. The setting clamp remains covered by unit tests on all platforms. Oversized return packets are tested separately. |
| Oversized replies | Responses whose SOCKS header plus payload exceeds the limit are dropped; subsequent valid traffic still works. |
| Shared-reader overflow | A full 16-packet association queue drops overflow and does not block another association. Covered deterministically by `TestSocksUDPSharedRoutingAndOverflow`. |
| Invalid settings | Zero, negative, and malformed size/connection/deadline settings fall back to defaults; oversized size settings clamp. |
| Existing protocol limits | Malformed/unassociated packets and fragmentation are rejected; source-port conflicts and the one-pending-association-per-IP rule remain covered. |

The association and destination limits are per listener and per association respectively; they are not a process-wide socket or memory limit. Each association retains at most two outgoing sockets (IPv4 and IPv6) until it closes. Destination expiry releases tracking capacity; incoming replies do not create new tracking entries.

Local Linux/ARM64 validation passed: `CHISEL_UDP_STRESS=1 GOMAXPROCS=4 go test -race ./...`, `go vet ./...`, and `git diff --check`.

## Reproduce

Run the benchmark without race instrumentation:

```sh
GOMAXPROCS=4 go test ./test/e2e -run "^$" -bench '^BenchmarkSocksUDP/(direct|forward|reverse)/bytes=(1200|8192)/clients=(1|16)$' \
  -benchmem -benchtime=1s -count=3
```

Run the boundary and recovery tests:

```sh
go test -race ./test/e2e -run '^TestSocksUDP(AssociationLimit|DestinationLimit|PacketSizeLimit|TransferPort|SourcePortSurvivesIdle)$'
go test -race ./share/tunnel -run '^TestSocksUDP(LimitSettings|SharedRoutingAndOverflow|ExitLimitsAndRecovery)$'
```

Run the opt-in overload test:

```sh
CHISEL_UDP_STRESS=1 GOMAXPROCS=4 go test -v ./test/e2e -run "^TestSocksUDPOverload$"
```

Ordinary `go test ./...` runs the limit tests and skips the overload workload. Set `CHISEL_UDP_STRESS=1` to include it. Performance numbers above come from runs without the race detector; run the full suite with `-race` to check concurrency correctness.
