# Using SOCKS5 UDP with Chisel

Use `udpsocks` to carry both TCP and UDP traffic through a Chisel SOCKS5 proxy. It supports TCP CONNECT and UDP ASSOCIATE, including reverse tunnels.

Both Chisel peers need builds that include `udpsocks`. To build this working tree, run `go build -o chisel .` and use the resulting binary on each machine (build separately for different operating systems or architectures).

## Choose your listener

| Remote | Listener location | Listening sockets | Supported commands |
| --- | --- | --- | --- |
| `1080:socks` | Chisel client | TCP 1080 | CONNECT |
| `1080:udpsocks` | Chisel client | TCP 1080 and UDP 1080 | CONNECT and UDP ASSOCIATE |
| `R:1080:udpsocks` | Chisel server | TCP 1080 and UDP 1080 | CONNECT and UDP ASSOCIATE |

Bare `udpsocks` defaults to `127.0.0.1:1080`. Use `5000:udpsocks` to change the port, or `192.168.1.10:5000:udpsocks` to choose an interface. IPv6 works too: quote the remote, for example `'[::1]:1080:udpsocks'`.

TCP and UDP always use the same configured interface and port number. One UDP relay socket serves all associations: Chisel does not allocate a random relay port for each client. Port zero is rejected. This applies to the exposed relay; normal outgoing UDP sockets and application source ports can still use operating-system-assigned ports.

## Forward proxy: traffic exits through the server

The path is:

```text
SOCKS application → client TCP/UDP :1080 → Chisel tunnel → server → destination
```

On the server, create `users.json`. Replace `CHANGE_ME` with your own password:

```json
{
  "proxyuser:CHANGE_ME": ["^socks$"]
}
```

Generate a persistent server key once, then start the server:

```sh
chisel server --keygen chisel-key.pem
chisel server --keyfile chisel-key.pem --port 9312 --socks5 --authfile users.json
```

Copy the fingerprint printed by the server. On the client machine, replace `SERVER_FINGERPRINT`, `server.example.com`, and the password:

```sh
chisel client --fingerprint 'SERVER_FINGERPRINT' --auth 'proxyuser:CHANGE_ME' \
  http://server.example.com:9312 1080:udpsocks
```

Point the SOCKS application at **127.0.0.1:1080 on the client machine**. For example, this checks TCP CONNECT:

```sh
curl --socks5-hostname 127.0.0.1:1080 https://example.com
```

The SOCKS application must support SOCKS5 UDP ASSOCIATE to send UDP. Configuring a browser or an HTTP-only SOCKS client does not automatically tunnel its UDP traffic. Chisel tunnel credentials authenticate the Chisel client; the local SOCKS listener uses no SOCKS username/password authentication.

## Reverse proxy: traffic exits through the client

Use this when applications on the server should access destinations reachable from the client's network:

```text
SOCKS application → server TCP/UDP :1080 → Chisel tunnel → client → destination
```

On the server, create `reverse-users.json`:

```json
{
  "exituser:CHANGE_ME": ["^R:127\\.0\\.0\\.1:1080$"]
}
```

Start the server using the persistent key generated above:

```sh
chisel server --keyfile chisel-key.pem --port 9312 --reverse \
  --authfile reverse-users.json
```

Connect from the machine that should act as the exit:

```sh
chisel client --fingerprint 'SERVER_FINGERPRINT' --auth 'exituser:CHANGE_ME' \
  http://server.example.com:9312 R:1080:udpsocks
```

Point SOCKS applications at **127.0.0.1:1080 on the server machine**. Reverse mode needs `--reverse` on the server; it does not require `--socks5` there. The client supplies the SOCKS exit.

For forward mode, both `socks` and `udpsocks` use the authfile permission `socks`. For reverse mode, permission matches the server listener address, such as `R:127.0.0.1:1080`. Changing the listener interface or port also requires updating that reverse permission.

## Try UDP with an independent client

This example uses [PySocks](https://pypi.org/project/PySocks/), which performs the SOCKS negotiation and UDP encapsulation itself.

First, run a UDP echo endpoint on the **exit machine**: the server for forward mode, or the client for reverse mode. Leave it running:

```sh
python3 - <<'PY'
import socket

with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as echo:
    echo.bind(('127.0.0.1', 9999))
    print('UDP echo listening on 127.0.0.1:9999', flush=True)
    while True:
        data, peer = echo.recvfrom(65535)
        echo.sendto(data, peer)
PY
```

On the **SOCKS listener machine**, install PySocks in a virtual environment:

This needs Python's virtual environment support (`python3-venv` on Debian/Ubuntu). On Windows, use `.venv\Scripts\python.exe` instead of `.venv/bin/python`.

```sh
python3 -m venv .venv
.venv/bin/python -m pip install PySocks==1.7.1
```

Then send a datagram through the proxy:

```sh
.venv/bin/python - <<'PY'
import socket
import socks

with socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM) as client:
    client.set_proxy(socks.SOCKS5, '127.0.0.1', 1080)
    client.settimeout(5)
    client.bind(('127.0.0.1', 0))
    payload = b'hello through udpsocks'
    client.sendto(payload, ('127.0.0.1', 9999))
    response, source = client.recvfrom(65535)
    assert response == payload, (response, source)
    print('UDP round trip succeeded:', response.decode())
PY
```

The destination `127.0.0.1:9999` is reached from the exit machine. The application's source port may be allocated dynamically; the Chisel relay remains on port 1080.

The repository also includes interoperability tests using the independent Go client [txthinking/socks5](https://github.com/txthinking/socks5):

```sh
go test -v ./test/e2e -run TestSocksUDPThirdPartyClient
```

Those tests cover forward and reverse UDP, IPv4, IPv6, hostname destinations, multiple packet sizes, and the configured relay port.

For measured throughput, latency, overload behavior, and boundary tests, see the [performance and limit test report](udpsocks-performance.md). It includes commands to repeat the benchmarks and tests.

## Interfaces, firewalls, and Docker

Keep the SOCKS listener on loopback when applications run on the same machine. To accept applications on a trusted network, specify a reachable interface, for example `192.168.1.10:1080:udpsocks`. The listener has no SOCKS authentication, so control access to that address and port.

Allow **both TCP and UDP** to the configured SOCKS port when applications connect across a firewall. Between the two Chisel peers, allow the Chisel transport port (9312 in these examples); SOCKS UDP travels inside the tunnel, so no separate UDP tunnel port is required between peers.

UDP ASSOCIATE advertises the concrete local IP accepting the TCP control connection. The application must be able to reach that IP over UDP. Linux Docker listeners can use `--network host` and `0.0.0.0:1080:udpsocks` to avoid advertising an isolated container IP. With bridge networking, mapping both `1080:1080/tcp` and `1080:1080/udp` is necessary, but the advertised container IP must also be reachable; port mappings alone do not guarantee this.

## Troubleshooting and limits

| Symptom | Check |
| --- | --- |
| TCP works, UDP ASSOCIATE is rejected | Use `udpsocks`, upgrade both Chisel peers, and check logs for a UDP bind failure. Forward mode also requires server `--socks5`. |
| UDP port is already occupied | Free the configured UDP port and restart the proxy. Chisel retains TCP service and rejects ASSOCIATE; it never switches to another relay port. |
| Association succeeds but packets time out | Check the destination from the exit machine, UDP firewall rules, and reachability of the advertised relay IP. |
| Another client sharing an IP cannot associate | Only one association with an unknown UDP source port may wait per source IP. Send the first client's first datagram before opening the next association, or use clients that declare distinct source ports. |
| UDP stops after reconnecting | Keep the TCP control connection open. Losing that connection or the SSH tunnel ends the association; create a new UDP ASSOCIATE after reconnecting. |
| Large or fragmented datagrams disappear | Fragmented SOCKS UDP packets are unsupported. The default packet limit is 9012 bytes, including the SOCKS UDP header. |

On the exposed relay, only client datagrams from the TCP peer's IP are accepted. A declared source port reserves that endpoint; otherwise, the first valid datagram pins the association to its source port. DNS names in UDP destination headers are resolved at the exit machine.

Each association uses at most one outgoing socket per address family, retaining its source port across destination changes and idle periods until the association closes. Replies may use a different source address or UDP port, as protocols such as TFTP require; the SOCKS reply header carries the actual source address and port. Incoming replies do not allocate additional destination tracking entries.

Defaults are 100 associations per listener, 100 destinations per association, and a 15-second idle timeout for destination tracking. Expired destinations free capacity and are resolved again on the next packet; the outgoing sockets remain available until the association closes. Each association queues up to 16 outbound datagrams and drops overflow. Tune these with `CHISEL_UDP_MAX_CONNS`, `CHISEL_UDP_DEADLINE`, and `CHISEL_UDP_MAX_SIZE` on the relevant peers.

Chisel caps SOCKS UDP packets at 65507 bytes, including the SOCKS header. The operating system can impose a smaller send limit: macOS defaults to 9216 bytes (`net.inet.udp.maxdgram`), which accommodates Chisel's 9012-byte default. Increasing `CHISEL_UDP_MAX_SIZE` does not increase the operating system's limit.

SOCKS BIND, UDP fragmentation, and `stdio:udpsocks` are unsupported. UDP is best-effort: queue and resource limits can drop packets, and reconnecting does not replay them.
