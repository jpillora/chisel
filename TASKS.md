# TASKS

a [meads](https://github.com/jpillora/meads) (`md`) managed task log

* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T11:52:45Z

## 1. Keepalive ping has no timeout - dead connections are never detected

* status: closed
* priority: P1
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T10:22:12Z

### Problem

`share/tunnel/tunnel.go:178` — keepAliveLoop calls `sshConn.SendRequest("ping", true, nil)` which blocks until the peer replies. On a dead TCP path (OS sleep/wake, NAT timeout, server hard reboot) no RST arrives, so the request blocks for the kernel retransmit timeout (15+ min). The tunnel looks connected but every OpenChannel hangs; the client never reconnects.

This is the most-reported problem in the tracker.

### Fix

- Race SendRequest against a timer (keepalive interval or a `CHISEL_PING_TIMEOUT` env), close the ssh conn on timeout so the reconnect loop kicks in
- Replace the `time.Sleep` loop with a stoppable ticker
- Review/merge PR #581 (current attempt, has tests)

### Refs

- [#445](https://github.com/jpillora/chisel/issues/445) — Keepalive timers for disconnected chisel client
- [#560](https://github.com/jpillora/chisel/issues/560) — Client keeps disconnecting every 3-5 minutes
- [#579](https://github.com/jpillora/chisel/issues/579) — Cant reconnect after server reboot
- [PR #581](https://github.com/jpillora/chisel/pull/581) — add timeout to keepAliveLoop (preferred fix)
- [PR #583](https://github.com/jpillora/chisel/pull/583), [PR #488](https://github.com/jpillora/chisel/pull/488), [PR #442](https://github.com/jpillora/chisel/pull/442), [PR #481](https://github.com/jpillora/chisel/pull/481) — earlier attempts

## 2. Authfile fsnotify watcher misses rename/truncate updates; no debounce

* status: closed
* priority: P1
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T10:26:30Z

### Problem

`share/settings/users.go:100-121` — addWatchEvents watches the file path and only reacts to `fsnotify.Write`. Editors that write via tmp+rename (vim), truncate-then-write scripts, and Kubernetes configmap symlink swaps either kill the watch (inode replaced) or emit Create/Rename events that are ignored. A truncate+write can also race the reader into loading a half-written file: reload fails and stale users stay active until restart.

### Fix

- Watch the parent directory filtered by filename
- Re-add the watch after Remove/Rename events
- Debounce (~100ms) and only swap users when the JSON parses
- Review/merge PR #587 (adds debounce + path normalization)

### Refs

- [#493](https://github.com/jpillora/chisel/issues/493) — Intermittent authentication failure after updating users.json (k8s, restart required)
- [#485](https://github.com/jpillora/chisel/issues/485) — Version mismatch when using auth file (related reports)
- [PR #587](https://github.com/jpillora/chisel/pull/587) — debounce and improve config file watcher

## 3. UDP exit node: flows beyond 100 are permanently broken and leak

* status: closed
* priority: P1
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T10:37:22Z

### Problem

`share/tunnel/tunnel_out_ssh_udp.go:65-72` — handleWrite: when `udpConns.len() > maxConns` (100), the new conn is still dialed and stored in the map but no handleRead goroutine is spawned. Consequences:

1. Responses for that flow are never read — silent blackhole
2. The entry is never removed (removal only happens in the handleRead defer), so the map never drops below the cap and **all** subsequent new flows stay broken until the SSH channel closes
3. Memory grows unbounded

Affects busy DNS/QUIC tunnels.

### Fix

Review/merge PR #515 (fixes removal, makes the cap configurable). Longer term, implement the TODO at line 62: replace goroutine-per-flow with a periodic idle sweep.

### Refs

- [PR #515](https://github.com/jpillora/chisel/pull/515) — fix the udpConns map does not release new conns when its length is over 100
- [#406](https://github.com/jpillora/chisel/issues/406) — Improve udp tunnel stability
- [#456](https://github.com/jpillora/chisel/issues/456) — Exposing a Minecraft Bedrock Server (UDP)

## 4. Security: SOCKS channels bypass per-user ACL

* status: closed
* priority: P1
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T10:31:28Z

### Problem

`share/tunnel/tunnel_out_ssh.go:50` skips the ACL for socks channels:

```go
if t.Config.ACL != nil && !socks && !t.Config.ACL(hostPort) {
```

With `--socks5` and `--authfile` both enabled, a modified client can open a `socks` channel directly — without declaring a socks remote in its config — and get unrestricted egress through the SOCKS server, regardless of its authfile address list. The config-time check (`server/server_handler.go:113-122`) only validates remotes the client chooses to declare; channels are not required to match declared remotes.

Note: `settings.Remote.UserAddr()` (`share/settings/remote.go:220-225`) returns `:` for socks remotes, so authfiles cannot even express "allow socks" cleanly — users allow it by accident via unanchored regexes.

Channel ACLs were added in commit 44310b6 but left this hole.

### Fix

- Subject socks channels to the ACL with a well-known token (e.g. require `HasAccess("socks")`)
- Make `UserAddr()` return `socks` for socks remotes
- Document the migration for existing authfiles

### Refs

- [#563](https://github.com/jpillora/chisel/issues/563) — Are Unauthorized clients allowed to connect and use Chisel server?
- [#518](https://github.com/jpillora/chisel/issues/518) — How could configure authfile when using remote socks

## 5. BindRemotes leaks bound listeners on partial failure

* status: closed
* priority: P2
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T11:04:40Z

### Problem

`share/tunnel/tunnel.go:148-163` — NewProxy binds each remote eagerly (tcp/udp listen in `tunnel_in_proxy.go:45-70`). If remote N fails to bind after remotes 1..N-1 succeeded, BindRemotes returns the error without closing the earlier proxies, so their sockets stay bound for the process lifetime. On the server (reverse mode) a failed client config can orphan ports forever.

The `CanListen()` precheck in server_handler.go shrinks the window but is TOCTOU (another bind can win between precheck and the real bind) and does not cover resolve errors.

### Fix

Add `Proxy.Close()` and close `proxies[0..i-1]` on error before returning.

### Refs

- [#492](https://github.com/jpillora/chisel/issues/492) — multiple sockets bind to the same address (related)

## 6. Server auth: session map leak, panic race, literal %s error, timing-unsafe compare

* status: closed
* priority: P2
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T11:07:30Z

### Problem

All in the server auth path (`server/server.go` authUser + `server/server_handler.go` handleWebsocket):

**Session map leaks** — authUser stores the user in `s.sessions` keyed by SSH SessionID (`server.go:199-215`); `server_handler.go:67-77` deletes it only after a successful NewServerConn:

- Client drops after PasswordCallback but before the handshake completes — entry never deleted
- Config request timeout path (`server_handler.go:83-89`) returns without Del

Unbounded (if slow) growth on a public server.

**Panic race** — if `users.Len()==0` during auth (allow-all, nothing stored) and an authfile reload makes `Len()>0` before the check at `server_handler.go:69-74`, the handler panics ("bug in ssh auth handler"). net/http recovers it, but the connection dies with a stack trace in the log.

**authUser nits** (`server.go:207-209`):

- `errors.New("Invalid authentication for username: %s")` — the `%s` is literal, never formatted
- `user.Pass != string(password)` is not constant time — a (mild) timing oracle on a public endpoint

### Fix

- Delete the session entry via defer in handleWebsocket; replace the panic with a logged close
- Better: return the user via `ssh.Permissions.Extensions` from PasswordCallback instead of a side map — removes the map, the race, and the TODO at `server.go:212` entirely
- Fix the error string; use `crypto/subtle.ConstantTimeCompare`; add tests

## 8. Auth user store: --auth validation foot-guns + reload semantics (#549)

* status: closed
* priority: P2
* type: bug
* created: 2026-06-10T10:22:47Z
* updated: 2026-06-12T11:17:02Z

### Problem

Three related defects in how the user store is built and reloaded:

**(a) Silently disabled** — `settings.ParseAuth` (`share/settings/user.go:10-16`) returns empty strings when the value has no colon. `server/server.go:71-77` only adds the user when `Name != ""`, so `chisel server --auth secretword` starts with **no authentication** and no warning. The client likewise silently sends empty creds.

**(b) --auth user silently dropped** — when `--auth` is combined with `--authfile`, the --auth user is added to the same UserIndex that authfile reloads `Reset()` (`share/settings/users.go:61-69` and `:157`), so the first file reload deletes the --auth user and that client can no longer connect.

**(c) Reload does not affect connected clients** — the per-connection ACL closure captures the `*settings.User` object at handshake time (`server/server_handler.go:146-148`: `tunnelConfig.ACL = user.HasAccess`). After a reload, removed users keep tunneling until they disconnect, and updated Addrs lists are not applied to existing connections.

### Fix

- Make auth strings without a colon a fatal config error on both sides
- Keep the --auth user separate from file-sourced users, or re-add it after each reload
- Resolve the user by name from `s.users` at channel-open time so current Addrs always apply; optionally track active `ssh.Conn` per user and `Close()` the ones removed/changed on reload (#549 explicitly expects disconnection). At minimum document the behavior

Pairs with the external-auth (`--authurl`) idea (task 28).

### Refs

- [#549](https://github.com/jpillora/chisel/issues/549) — clients are not disconnected when auth file is replaced

## 10. Client connect loop: exit 0 on give-up, 100ms backoff Min, robustness nits

* status: closed
* priority: P2
* type: bug
* created: 2026-06-10T10:23:15Z
* updated: 2026-06-12T11:11:58Z

### Problem

All in `client/client_connect.go` connectionLoop:

**Exit code** (`:49-52`) — after exhausting `--max-retry-count` the loop breaks, calls `c.Close()` and returns nil, so the process exits 0. Scripts and systemd units cannot distinguish "tunnel worked and was interrupted" from "never connected".

**Backoff Min** (`:22`) — `backoff.Backoff{Max: MaxRetryInterval}` leaves Min at the library default of **100ms**, so a down server gets hit at 100ms/200ms/400ms... by every client — a reconnect stampede when a popular server restarts. Related wart: `client/client.go:78` silently raises any `MaxRetryInterval < 1s` to 5 minutes.

**Robustness nits**:

- `:33` calls `err.Error()` before the nil check at `:37` — connectionOnce currently never returns `(true, nil)` because `ssh.Conn.Wait()` returns io.EOF on clean close, but any future change makes this a nil-pointer panic
- `:46` — `c.Infof(msg)` passes pre-built text as the format string; any literal `%` garbles output

### Fix

- Return an error when attempts are exhausted so `main.go` log.Fatal exits non-zero; keep exit 0 for ctx-cancelled shutdown
- Set a saner backoff Min (e.g. 1s) and/or add `--min-retry-interval`; warn or document the 5-minute floor
- Guard `err != nil` first; use `c.Infof("%s", msg)` (grep for the pattern elsewhere)

### Refs

- [PR #537](https://github.com/jpillora/chisel/pull/537) — Fix client backoff
- [#579](https://github.com/jpillora/chisel/issues/579) — Cant reconnect after server reboot (likely related)

## 12. Uppercase /UDP remote suffix parses but later fails as unknown proto

* status: closed
* priority: P3
* type: bug
* created: 2026-06-10T10:23:15Z
* updated: 2026-06-12T11:23:56Z

### Problem

`share/settings/remote.go:154-163` — the l4Proto regex is case-insensitive (`(?i)/(tcp|udp)$`) but the returned proto keeps the original case, while every comparison expects lowercase (`remote.go:114-127`, `tunnel_in_proxy.go:48-67`, `tunnel_out_ssh.go:42`). So `chisel client SERVER 1.1.1.1:53/UDP` decodes successfully, then dies at listen time with "unknown local proto". The head is lowercased but the proto is not — inconsistent.

### Fix

`strings.ToLower` the proto in L4Proto; add a remote_test case.

## 14. NewServer kills the host process with log.Fatal

* status: closed
* priority: P3
* type: bug
* created: 2026-06-10T10:23:15Z
* updated: 2026-06-12T11:25:35Z

### Problem

`server/server.go:79-111` — the key-loading paths call `log.Fatalf`/`log.Fatal` (failed to read key file, invalid key, failed to generate key, failed to parse key) inside NewServer, which otherwise returns `(*Server, error)`. Chisel is also consumed as a library, and log.Fatal kills the embedding process and skips deferred cleanup.

### Fix

Return errors; `main.go` already log.Fatals on the returned error.

### Refs

- [#542](https://github.com/jpillora/chisel/issues/542) — Usage in go code?
- [#497](https://github.com/jpillora/chisel/issues/497) — there is no stop function?

## 17. Set websocket read limits (pre-auth memory DoS hardening)

* status: closed
* priority: P2
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T10:59:53Z

### Problem

`share/cnet/conn_ws.go:25-53` reads entire websocket messages into memory (`ReadMessage`) and buffers the remainder, with no size cap; gorilla/websocket defaults to unlimited message size. On the server this happens **before SSH authentication**, so an unauthenticated peer can send a multi-GB frame and OOM the process.

### Fix

SSH packets are <= ~35KB, so call `SetReadLimit` (e.g. 64KB) on both server (`server/server_handler.go` after Upgrade) and client (`client/client_connect.go` after Dial), or switch wsConn.Read to NextReader-based streaming. The `WS_BUFF_SIZE` env already tunes buffer sizes; a read limit closes the abuse case.

## 18. Graceful shutdown: handle SIGTERM and use http.Server.Shutdown

* status: closed
* priority: P2
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T10:56:34Z

### Problem

- `share/cos/common.go:12-22` InterruptContext only registers `os.Interrupt`; SIGTERM (docker stop, kubernetes) takes the default kill path so context-based cleanup never runs (the code comment even wonders "windows compatible?")
- `share/cnet/http_server.go` is documented as "adds graceful shutdowns" but GoServe calls `Close()` (immediate) when the ctx ends; the `listenErr` field is dead code

### Fix

Add `syscall.SIGTERM` (unix) and make a second signal force-exit. Use `http.Server.Shutdown` with a grace period, then Close.

### Refs

- [PR #564](https://github.com/jpillora/chisel/pull/564) — Reuse existing function to implement graceful shutdown logic (review alongside)

## 19. Half-close support + dial-failure propagation through tunnels

* status: closed
* priority: P2
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T10:52:18Z

### Problem

`cio.Pipe` (`share/cio/pipe.go:9-30`) closes **both** directions as soon as either io.Copy finishes, so TCP half-close semantics (send, shutdown(WR), read reply) break across the tunnel, and a FIN from one end becomes a full teardown.

Related: `share/tunnel/tunnel_out_ssh.go:87-95` accepts the SSH channel **before** dialing the target, so inbound TCP clients see a successful connection even when the target is down/refusing. Also `net.Dial` there has no timeout/context (hangs the channel for the OS dial timeout).

### Fix

Design pass:

- CloseWrite-aware Pipe (`ssh.Channel` and `*net.TCPConn` both have CloseWrite)
- Reject channel on dial failure (or dial-before-accept)
- Context-aware dial with timeout
- e2e coverage in test/e2e

### Refs

- [#535](https://github.com/jpillora/chisel/issues/535) — Non graceful closing of remote connection
- [#447](https://github.com/jpillora/chisel/issues/447) — when channel is closed, outbound always by use
- [PR #536](https://github.com/jpillora/chisel/pull/536) — Half closing of connections when CloseWrite() is available
- [PR #548](https://github.com/jpillora/chisel/pull/548) — Add CloseWrite() to rwcConn (SOCKS path)
- [PR #538](https://github.com/jpillora/chisel/pull/538) — TCP reset on remote connection failure

## 20. CI/release hygiene: scope permissions, fix docker version stamping

* status: closed
* priority: P2
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T10:45:45Z

### Problem

- `.github/workflows/ci.yml` sets `permissions: write-all` for all jobs on every push AND pull_request (also causes duplicate runs on PR branches)
- `.github/Dockerfile` stamps the version via `git describe --abbrev=0 --tags` during docker build; the release_docker checkout has no tags fetched (no `fetch-depth: 0`), so docker images get wrong/empty versions

### Fix

Scope to least privilege (`contents: read` for test, `contents: write` only on release jobs). Review/merge PR #584: goreleaser-built docker images (multi-arch, GHCR + Docker Hub), single release job, scoped permissions.

### Refs

- [#417](https://github.com/jpillora/chisel/issues/417) — Docker/github version string mismatch
- [PR #584](https://github.com/jpillora/chisel/pull/584) — Improve Docker: use GoReleaser for images

## 21. Tracker triage: close stale dependabot PRs and resolved issues

* status: closed
* priority: P2
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T10:40:41Z

### Stale dependabot PRs

Superseded by the dependency updates in [PR #568](https://github.com/jpillora/chisel/pull/568) (2025-09) and [PR #578](https://github.com/jpillora/chisel/pull/578) (2026-02) — close:
[#448](https://github.com/jpillora/chisel/pull/448), [#449](https://github.com/jpillora/chisel/pull/449), [#450](https://github.com/jpillora/chisel/pull/450), [#451](https://github.com/jpillora/chisel/pull/451), [#467](https://github.com/jpillora/chisel/pull/467), [#470](https://github.com/jpillora/chisel/pull/470), [#478](https://github.com/jpillora/chisel/pull/478), [#496](https://github.com/jpillora/chisel/pull/496), [#513](https://github.com/jpillora/chisel/pull/513), [#516](https://github.com/jpillora/chisel/pull/516), [#517](https://github.com/jpillora/chisel/pull/517).

The dependabot config evidently rots — consider grouped monthly updates or renovate ([#559](https://github.com/jpillora/chisel/issues/559); [#452](https://github.com/jpillora/chisel/issues/452) reported confusing "fake pushes").

### Issues closeable now

- [#561](https://github.com/jpillora/chisel/issues/561) — Heroku demo dead (close via docs task 23)
- [#585](https://github.com/jpillora/chisel/issues/585) — fixed by [PR #586](https://github.com/jpillora/chisel/pull/586)
- [#445](https://github.com/jpillora/chisel/issues/445), [#560](https://github.com/jpillora/chisel/issues/560) — fold into keepalive fix (task 1)
- [#541](https://github.com/jpillora/chisel/issues/541) — unsubstantiated buffer-overflow claim; no evidence provided; close with explanation
- [#504](https://github.com/jpillora/chisel/issues/504), [#550](https://github.com/jpillora/chisel/issues/550), [#485](https://github.com/jpillora/chisel/issues/485) — close via version fallback (task 22)
- [#498](https://github.com/jpillora/chisel/issues/498) — close via docs task 23

### Duplicate PRs

Keepalive fixes [PR #581](https://github.com/jpillora/chisel/pull/581) / [PR #488](https://github.com/jpillora/chisel/pull/488) / [PR #442](https://github.com/jpillora/chisel/pull/442) — pick #581, close the rest with thanks.

## 22. Version: fall back to debug.ReadBuildInfo when ldflags absent

* status: closed
* priority: P2
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T11:02:07Z

### Problem

`go install github.com/jpillora/chisel@latest` produces BuildVersion `0.0.0-src`, which then logs "Client version (0.0.0-src) differs from server version (...)" on every connect — recurring user confusion.

### Fix

In `share/version.go`, when BuildVersion is the default, fall back to `runtime/debug.ReadBuildInfo().Main.Version` (gives v1.x.y for module installs). Skip the mismatch warning in `server/server_handler.go:104-111` when either side reports a dev/unknown version.

### Refs

- [#504](https://github.com/jpillora/chisel/issues/504) — Client version (0.0.0-src) differs from server version (v1.9.1)
- [#550](https://github.com/jpillora/chisel/issues/550) — Client version difference on Kali Linux
- [#485](https://github.com/jpillora/chisel/issues/485) — Version mismatch when using auth file

## 23. Docs refresh: CLI help + README (wrong defaults, dead demo, install cmd, proxy creds)

* status: closed
* priority: P3
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T11:30:50Z
* Demo section: chisel-demo.herokuapp.com is dead (Heroku free tier removed; issue #561). Replace with a fly.io demo (example/fly.toml already exists) or drop the section.

### CLI help text (main.go, rendered into README via md-tmpl)

- `main.go:317` claims "remote-host defaults to 0.0.0.0 (server localhost)" — the code defaults RemoteHost to `127.0.0.1` (`share/settings/remote.go:110-112`), which is what "server localhost" actually means. Fix and re-render
- `--keyfile` help is misleading: the "inline base64" example (`chisel server --keygen - | base64`) is wrong because keygen already outputs a `ck-...` base64 string; no extra base64 step is needed — [#498](https://github.com/jpillora/chisel/issues/498), [PR #461](https://github.com/jpillora/chisel/pull/461)
- Sweep for accuracy: "fallsback" (`main.go:105`), ragged tab indentation in the --fingerprint paragraph

### README

- Demo section: chisel-demo.herokuapp.com is dead (Heroku free tier removed). Replace with a fly.io demo (`example/fly.toml` exists) or drop the section — [#561](https://github.com/jpillora/chisel/issues/561)
- Verify install one-liner `curl https://i.jpillora.com/chisel! | bash` — [PR #562](https://github.com/jpillora/chisel/pull/562) claims broken
- Demo text says `--proxy` where the flag docs say `--backend`; `main.go:189-190` registers both names for the same field — document the alias — [PR #556](https://github.com/jpillora/chisel/pull/556)
- Document that `--proxy` credentials must be URL-encoded (a `#` in the password truncates the URL) — [#396](https://github.com/jpillora/chisel/issues/396)
- Dead links/badges: microbadger badge, Google App Engine tracker (code.google.com)
- Requested examples: TLS setup walkthrough ([#533](https://github.com/jpillora/chisel/issues/533)), reverse socks + authfile ([#518](https://github.com/jpillora/chisel/issues/518)), cloudflare/CDN fronting notes ([#490](https://github.com/jpillora/chisel/issues/490))
- Document the AUTH env var for both sides and the CHISEL_* env knobs (WS_TIMEOUT, SSH_TIMEOUT, UDP_MAX_SIZE, UDP_DEADLINE, CONFIG_TIMEOUT, SSH_WAIT) — currently undocumented

## 24. Accept socks5:// scheme in client --proxy

* status: closed
* priority: P3
* type: task
* created: 2026-06-10T10:23:53Z
* updated: 2026-06-12T11:33:12Z

### Problem

`client/client.go:265-294` setProxy accepts only `socks://` and `socks5h://` and rejects `socks5://` — the most common spelling.

### Fix

Add `socks5` to the allowed schemes (same SOCKS5 dialer). Note in help/docs: all variants resolve DNS via the proxy (`golang.org/x/net/proxy.SOCKS5`), so socks5 vs socks5h semantics are identical here. One-liner plus a test and help-text update.

### Refs

- [#474](https://github.com/jpillora/chisel/issues/474) — Support socks5:// protocol as client proxy protocol

## 25. Trivial sweep: typos, tiny community PRs, server_listen.go dead code

* status: closed
* priority: P3
* type: task
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:36:27Z

### server_listen.go dead code

- `server/server_listen.go:41-43`: the "LetsEncrypt will attempt to connect to your domain on port 443" warning is computed inside the hasKeyCert branch but guarded by hasDomains — impossible, since `hasDomains && hasKeyCert` already returned an error at line 27. Move the warning into the hasDomains path (port != 443) where it was clearly intended
- Line 56 `if err == nil` is always true (err checked at line 47) — [PR #546](https://github.com/jpillora/chisel/pull/546) removes it; merge/credit

### Pending trivial PRs to merge or replicate

- [PR #588](https://github.com/jpillora/chisel/pull/588) / [PR #575](https://github.com/jpillora/chisel/pull/575) — "forwaring" -> "forwarding" (`server/server_handler.go:126`, user-facing error message)
- [PR #528](https://github.com/jpillora/chisel/pull/528) — typo fix
- [PR #430](https://github.com/jpillora/chisel/pull/430) — add codespell to CI (prevents recurrence)

### Typos found in review

- "respresent" — `server/server.go:37`
- "recieved" — `share/cos/signal.go:27` (user-facing log line)
- "aquired" — `share/tunnel/tunnel_in_proxy_udp.go:181`
- "extacts" — `share/settings/remote.go:156`
- "faily" — `share/settings/remote.go:122`
- "successfuly" — `server/server_handler.go:135`
- "fallsback" — `main.go:105` (help text; overlaps docs task 23)

## 26. Auth/fingerprint hardening: legacy prefix match, unanchored regexes

* status: closed
* priority: P3
* type: task
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:39:13Z

### Items

**(a) Legacy MD5 fingerprint prefix match:** `client/client.go:233` uses `strings.HasPrefix(got, expect)` — a truncated legacy fingerprint like `a5:32` still "verifies" (matches 1 in 65k keys). Require the full 16-octet colon form before accepting.

**(b) Unanchored authfile regexes:** `share/settings/user.go:24-33` uses MatchString — an entry `10.0.0.1:80` also authorizes `210.0.0.1:8080` and `10.0.0.1:8000`; dots match any char. Anchoring by default is breaking, so at minimum document loudly with anchored examples — including that an empty-string entry (UserAllowAll) matches everything — and consider an opt-in flag or a startup warning for unanchored patterns.

(The socks `UserAddr() == ":"` quirk is covered by the SOCKS ACL bug, task 4.)

### Refs

- [#383](https://github.com/jpillora/chisel/issues/383) — Can chisel server be configured to allow only certain ports?
- [#543](https://github.com/jpillora/chisel/issues/543) — Using auth.json file with target verification

## 27. Build hygiene: Makefile flags, build tags, deprecated APIs, test nits

* status: closed
* priority: P4
* type: task
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:44:17Z

### Items

- Makefile linux/windows targets set `CGO_ENABLED=1` while goreleaser releases use 0 (static binaries are the point) — align to 0
- Makefile `dep` target uses deprecated `go get -u` for tools; use `go install tool@version`. `mkdir -p` runs on every make parse
- Old `//+build` tags (`share/cos/signal.go`, `signal_windows.go`, `pprof.go`) — switch to `//go:build`
- Deprecated `net.Error.Temporary()` at `share/tunnel/tunnel_in_proxy_udp.go:91`
- `client/client_test.go:39` uses log.Fatal instead of t.Fatal, and calls t.Fatal from the HTTP handler goroutine (illegal cross-goroutine use; hangs instead of failing)
- Dead code: `share/cio/pipe.go` pipeVis/vis const, `share/cnet/http_server.go` listenErr field, commented MeterRWC call (`tunnel_out_ssh.go:61`)
- e2e setup sleeps 50ms for client readiness (`test/e2e/setup_test.go:116`, acknowledged TODO) — add a readiness signal API to Client instead

## 28. External auth provider: --authurl webhook

* status: inprogress
* priority: P3
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:21:39Z

### Idea

[PR #582](https://github.com/jpillora/chisel/pull/582) implements `--authurl`: the server POSTs `{"username", "password"}` to an HTTP service; a 200 response with a JSON array of address regexes grants access, anything else denies.

### Decisions needed

- Response caching/TTL
- Request timeout + fail-closed semantics
- Secret/header for authenticating chisel to the auth service
- Interaction with live-ACL semantics (see task 8 — resolving the user per channel-open would make webhook auth consistent too)

Review the PR rather than reimplementing.

### Refs

- [PR #582](https://github.com/jpillora/chisel/pull/582) — Add support for server --authurl parameter
- [#476](https://github.com/jpillora/chisel/issues/476) — Feature request: support external authentication system
- [#574](https://github.com/jpillora/chisel/issues/574) — Integration for OTP / Std Logging

## 29. Observability: connection logs, /metrics, session introspection

* status: closed
* priority: P3
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:21:11Z

### Recurring asks

- See connected clients and their IPs — [#530](https://github.com/jpillora/chisel/issues/530)
- Log all connection attempts incl. failed auth — [#521](https://github.com/jpillora/chisel/issues/521)
- Show authenticated user on connect — [#501](https://github.com/jpillora/chisel/issues/501)
- Active-connection count endpoint for autoscaling — [#522](https://github.com/jpillora/chisel/issues/522), [PR #408](https://github.com/jpillora/chisel/pull/408)
- Prometheus /metrics — [PR #407](https://github.com/jpillora/chisel/pull/407)
- Connect/disconnect hooks — [#410](https://github.com/jpillora/chisel/issues/410), [PR #487](https://github.com/jpillora/chisel/pull/487)
- Client identifier — [#468](https://github.com/jpillora/chisel/issues/468)

### Minimal valuable step

Structured Info-level log on session open/close with user, remote IP, declared remotes (the server currently logs sessions only at Debug level — server_handler.go); failed auth at Info (currently Debugf, `server.go:208`).

Optional next: a /metrics endpoint (sessions, tunnels, bytes — ConnCount and Meter plumbing already exist) gated behind a flag, reusing the health/version switch in handleClientHandler.

## 30. HTTP fallback transport (SSE/long-poll) for websocket-hostile proxies

* status: inprogress
* priority: P3
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:17:32Z

### Idea

[#589](https://github.com/jpillora/chisel/issues/589) proposes an opt-in SSE+POST transport: long-lived GET for server->client, POST per frame for client->server, reducing to a net.Conn behind the `share/cnet` seam (like conn_ws.go / conn_rwc.go), keeping the single multiplexed SSH session, stdlib only, websocket stays the default.

### Why

Long-standing demand from environments that strip Upgrade headers ([#24](https://github.com/jpillora/chisel/issues/24), [#375](https://github.com/jpillora/chisel/issues/375)) and IDS-blocked deployments ([#507](https://github.com/jpillora/chisel/issues/507), [#432](https://github.com/jpillora/chisel/issues/432)).

### Considerations

Evaluate building on webdial vs the standalone implementation offered in the issue; buffering-proxy pathologies; auth/path configurability ([#566](https://github.com/jpillora/chisel/issues/566) custom URI pairs well).

### Refs

- [#589](https://github.com/jpillora/chisel/issues/589) — SSE transport for environments that block WebSocket upgrades

## 31. PROXY protocol support (server ingress + reverse-remote egress)

* status: inprogress
* priority: P4
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:47:23Z

### Idea

1. Accept PROXY protocol on the chisel server listener so the real client IP survives an L4 load balancer — affects the logging/ACL-by-IP asks
2. Optionally emit PROXY headers on reverse-remote connections so backend services behind the client see the original source address

Interacts with requestlog TrustProxy (`server/server.go:176-178`). Parsing must be optional and default-off (header injection risk on public listeners).

### Refs

- [#540](https://github.com/jpillora/chisel/issues/540) — [FEAT] Add Proxy Protocol support
- [PR #552](https://github.com/jpillora/chisel/pull/552) — PROXY v2 support (evaluate)

## 32. Unix domain socket remotes

* status: inprogress
* priority: P4
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:48:47Z

### Idea

Support unix sockets as local listeners and/or remote dial targets (e.g. tunneling a docker.sock or postgres socket).

Main blocker is remote syntax — `settings.Remote` parsing is colon-delimited (`share/settings/remote.go:43-133`); something like `unix:/path/to.sock` on either side needs explicit escaping rules. The dial/listen split in tunnel_in_proxy.go / tunnel_out_ssh.go is already proto-keyed (tcp/udp), so adding `unix` as an L4Proto is mechanically straightforward.

### Refs

- [#399](https://github.com/jpillora/chisel/issues/399) — [UDS] add unix domain support to chisel

## 33. Key/TLS management UX bundle

* status: inprogress
* priority: P4
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:49:43Z
* --keygen-json (PR #460): emit {key, fingerprint} JSON for automation; pairs with #499 (key automation ask).

### Bundle

- `--keygen-json`: emit `{key, fingerprint}` JSON for automation — [PR #460](https://github.com/jpillora/chisel/pull/460); pairs with [#499](https://github.com/jpillora/chisel/issues/499) (key automation)
- Encrypted PKCS#8 private keys + `--tls-keypass` for mTLS keys — [PR #565](https://github.com/jpillora/chisel/pull/565)
- Avoid plaintext proxy password: read from terminal/stdin — [PR #532](https://github.com/jpillora/chisel/pull/532)
- TLS fingerprint pinning for the transport layer, complementing the SSH `--fingerprint` — [#577](https://github.com/jpillora/chisel/issues/577)

Keep `test/e2e/env_key_test.go` green throughout — regression test for [#570](https://github.com/jpillora/chisel/issues/570) / [PR #571](https://github.com/jpillora/chisel/pull/571) (CHISEL_KEY env fix).

## 34. Assorted feature asks worth triaging

* status: inprogress
* priority: P4
* type: idea
* created: 2026-06-10T10:24:37Z
* updated: 2026-06-12T11:50:36Z

### Grab-bag from the last 100 issues

- Random/ephemeral local port (allow port 0 + report allocation) — [#434](https://github.com/jpillora/chisel/issues/434), [#410](https://github.com/jpillora/chisel/issues/410); isPort currently rejects 0 (`share/settings/remote.go:135-144`)
- Config-file driven client/server — [#393](https://github.com/jpillora/chisel/issues/393); CHISEL_MODE-style env selection for docker — [#436](https://github.com/jpillora/chisel/issues/436)
- WebSocket compression / zstd — [#529](https://github.com/jpillora/chisel/issues/529); gorilla supports permessage-deflate via EnableCompression
- IPv4/IPv6 dial preference flag — localhost resolving to ::1 surprises users when the service binds 127.0.0.1 only — [#544](https://github.com/jpillora/chisel/issues/544), [#479](https://github.com/jpillora/chisel/issues/479), [#520](https://github.com/jpillora/chisel/issues/520)
- Custom websocket endpoint path (--uri) — [#566](https://github.com/jpillora/chisel/issues/566); pairs with the SSE transport idea (task 30)
- Win7 support — [#576](https://github.com/jpillora/chisel/issues/576); infeasible on modern Go, document minimum OS in README instead

## 35. Dependabot rot: switch to grouped monthly updates or renovate

* status: inprogress
* priority: P4
* type: task
* created: 2026-06-12T10:40:33Z
* updated: 2026-06-12T11:52:45Z

The dependabot config produced 11 stale PRs that sat unmerged for years (closed 2026-06-12 in task 21). Decide between dependabot grouped monthly updates or renovate, then implement. Refs: [#559](https://github.com/jpillora/chisel/issues/559) (renovate suggestion), [#452](https://github.com/jpillora/chisel/issues/452) (confusing \"fake pushes\" from dependabot).
