# AGENTS.md

Context for AI agents and contributors working on the Sliver integration in this
chisel fork. Everything here was established by reading Sliver/Donut source and
by building and testing the extension locally — not assumed.

## Goal

Ship chisel as a **Sliver C2 extension** while keeping upstream chisel code
untouched, so the fork stays rebasable on `jpillora/chisel`.

The only added source file is `sliver.go`. Everything else is a manifest, two
Makefile targets, a README section, and one `.gitignore` line.

## Layout

| File             | Role                                                                                  |
| ---------------- | ------------------------------------------------------------------------------------- |
| `sliver.go`      | `//go:build cgo`; Sliver entrypoint ABI + task registry. **The only Go file we add.** |
| `extension.json` | Sliver extension manifest (Windows amd64/386 DLLs only)                               |
| `Makefile`       | `windows_binaries`, `sliver`, `extension` targets                                     |
| `README.md`      | "Sliver C2" section under Install                                                     |
| `.gitignore`     | `*.dll` (and existing `*.so`) so build artifacts are not committed                    |

## Build and install

On a Linux build host (Kali is fine):

```sh
sudo apt install -y gcc gcc-multilib mingw-w64
make sliver          # builds both DLLs, installs into ~/.sliver-client/extensions/chisel
make extension       # install only (DLLs already built)
```

- `windows_binaries` uses `CGO_ENABLED=1`, `-buildmode=c-shared`, and the mingw
  cross compilers (`x86_64-w64-mingw32-gcc-win32`, `i686-w64-mingw32-gcc-win32`).
- Output path is `./build/<version>/<commit>/bin/`. `DIR` embeds the commit SHA,
  so a new commit means a new directory — always run `make sliver` (which
  builds) rather than `make extension` after committing.
- Override the install path with `SLIVER_EXT=/some/path`.
- **Restart the Sliver client after installing** — extensions load at startup.

## Usage

```sh
chisel -- client -v 10.10.14.173:8080 R:1080:socks
chisel -- server -p 8080 --reverse
chisel -- list
chisel -- stop 1
```

The `--` matters: Sliver's shell parses its own flags first and only passes
positional arguments through.

## How it works

### The ABI

Sliver calls a native extension export as:

```c
int32_t entrypoint(char *buffer, uint32_t size, int32_t (*callback)(char *, int32_t))
```

- `buffer`/`size`: the raw argument string (not null-terminated, not
  `argv`-split). `sliver.go` does `C.GoStringN` then `strings.Fields`.
- `callback`: **only valid during the call**. Copy/report before returning;
  storing it for later use is a use-after-free.
- Return `0` for success, non-zero for failure.
- On `windows/386` the callback is `stdcall`, hence the `SLIVER_CALLBACK` macro
  in the cgo preamble. 64-bit Windows and Unix use the default convention.
- The manifest's `entrypoint` field must match the exported symbol name.

Source of truth: `sliverarmory/reflektor` `test/reflektor/fixtures/native_extension.c`
and `implant/sliver/extension/extension_unix.go` in `BishopFox/sliver`.

### Why the entrypoint returns immediately

Extensions are loaded **into the implant process** and invoked with
`CallExportWithArgs`. When the export returns, the RPC completes. So the
entrypoint starts the tunnel and returns; the tunnel keeps running on its own
goroutines.

`chisel/client.Start` and `chisel/server.StartContext` return immediately and do
not block, so no goroutine is needed to start a task. Only `Wait()` blocks, and
we deliberately never call it.

### Why not an alias

An `alias.json` manifest looks like it would work — it needs no source changes —
but it cannot, for two reasons established from Sliver source:

1. **Donut only resolves the `entrypoint` export for DLLs.** `donut.c:1542`:
   `if (fi.type == DONUT_MODULE_DLL && c->method[0] != 0) { if (!is_dll_export(...)) return DONUT_ERROR_DLL_FUNCTION; }`.
   Shipping a `.dll` without that export fails with
   `donut_generate returned error code: 12` (= `DONUT_ERROR_DLL_FUNCTION`,
   "Unable to locate DLL function provided"). Shipping an `.exe` skips the check,
   which is why `KriyosArcane/sliver-ligolo-ng` (plain `.exe` payloads) works.
2. **Aliases always kill the host process.** `alias_load.go` sends `Kill: true`
   unconditionally, and the implant runs `waitForCompletion(threadHandle)` then
   `cmd.Process.Kill()` (`taskrunner/task_windows.go`). A long-running tunnel
   either blocks the RPC until it times out, or returns and is immediately killed.
   There is no alias-side escape.

The extension path has neither problem.

### Why `Run()` is not called

`clientOptions.Run()` and `serverOptions.Run()` both end in `Wait()` and both
call `os.Exit` on failure paths (`log.Fatal`). Calling them would:

- block the entrypoint (Sliver RPC times out), and
- potentially kill the implant process.

`sliver.go` instead duplicates the setup portion of `Run()` — header parsing,
`AUTH` env fallback, `--hostname`/`--sni`, `HOST`/`PORT`/`KEY`/`KEY_FILE` env
defaults — then calls `NewClient`/`Start` or `NewServer`/`StartContext`.

**Maintenance note:** if upstream changes those defaults in `main.go`, mirror
the change in `startClient`/`startServer` in `sliver.go`. This is the one place
the fork can silently drift from upstream.

### Task registry

`sliverTask{id, kind, command, cancel, client, server}` in a mutex-guarded map,
ids from a monotonic counter.

- `list` — sorted `taskID=N type=... command=...`
- `stop <id>` — `cancel()`, then `client.Close()` / `server.Close()`, then
  removes the entry. Verified to release listening ports.

Nothing else stops a tunnel: there is no auto-expiry and no cleanup on session
end beyond the implant process itself dying.

## Verified behaviour

All of the following were run against a locally built `c-shared` library via a
`dlopen` harness on macOS:

| Call                              | Result                                         |
| --------------------------------- | ---------------------------------------------- |
| `--bogusflag`                     | `chisel error: ... unknown flag`, rc=1         |
| _(empty)_                         | `chisel error: usage: ...`, rc=1               |
| `client 127.0.0.1:1 R:1080:socks` | `task 1 started`, rc=0                         |
| `list`                            | shows the task                                 |
| `stop 1`                          | `task 1 stopped`, then `no running tasks`      |
| `stop 99`                         | `chisel error: no task 99`, rc=1               |
| server + client end-to-end        | both ports listening; after `stop` both closed |

The Windows DLL build itself has **not** been executed here — there is no mingw
cross compiler on the development host. Build and runtime on Windows are
unverified by this document; validate with `make sliver` on Kali and a real
session before relying on it.

## Known limitations

- **Windows only.** Linux was dropped deliberately: Sliver's Linux loader
  (`reflektor/native`) rejects Go `c-shared` images with
  `ErrGoSharedLibraryUnsupported`. The Linux `alias` path is also broken
  (`memfd` + `LD_PRELOAD` expects a shared library, not an ELF executable).
- **`make lint` can fail on newer Go** with `net.IPNet struct literal uses
unkeyed fields` in `pkg/proxy/netinfo/netinfo_unix.go`. This is pre-existing
  upstream, not caused by this fork; CI pins Go 1.25. Since `windows_binaries`
  depends on `lint`, that failure blocks `make sliver`.
- **`client version differs from server version`** warnings are harmless.
- **`Server cannot listen on R:127.0.0.1:1080=>socks`** means a previous task
  already holds that port on the chisel server. `list`, then `stop` it or pick a
  different port.
- **Orphaned tunnels from earlier alias experiments**: those ran in
  `notepad.exe` processes Sliver no longer tracks. They survive session kill and
  must be killed on the target directly.

## OPSEC notes

Trade-offs of running in-process, which an operator should weigh:

- chisel's code and strings live in the implant's memory; memory scanners see
  them in the C2 process.
- A second long-lived connection originates from the implant PID, trivially
  correlated with the C2 channel in netflow.
- A panic in chisel's per-stream goroutines is fatal to the implant, not just the
  tunnel. The entrypoint recovers panics on its own goroutine only.
- `cos.InterruptContext()` is **not** called, so chisel does not register
  SIGINT/SIGTERM handlers or `os.Exit` in the implant. `cos.GoStats()` is not
  started either. Keep it that way.
- Network fingerprint: the WebSocket subprotocol `chisel-v3`
  (`share/version.go`) is sent on every handshake and **is not configurable**.
  That is the strongest tell. Mitigations available without patching upstream:
  - `--header "User-Agent: ..."` (default is Go's `Go-http-client/1.1`)
  - `wss://` on 443 with a real certificate (`--tls-domain` on the server)
  - any URL path (the server upgrades whatever it receives), e.g. `/api/v2`
  - `--fingerprint <44-char>` instead of `--tls-skip-verify`
  - server `--backend http://example.com` to serve a decoy page to plain GETs
  - server `--auth` / `--authfile`; keep reverse socks on loopback only

If OPSEC outweighs convenience for an engagement, run chisel as a separate
process instead of an in-process extension.

## Do not

- Do not modify upstream chisel files to make Sliver work. Keep changes additive.
- Do not call `parsed.Run()`, `Wait()`, `log.Fatal`, or `os.Exit` from
  `sliver.go`.
- Do not store the Sliver callback pointer beyond the entrypoint call.
- Do not reintroduce `alias.json` or the `.exe` payload route — both hit the
  problems described above.
- Do not add Linux `.so` targets without first verifying the current Sliver
  Linux loader accepts Go c-shared images.
