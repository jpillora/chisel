# chisel

[![GoDoc](https://godoc.org/github.com/jpillora/chisel?status.svg)](https://godoc.org/github.com/jpillora/chisel)

Chisel is a fast TCP tunnel, transported over HTTP, secured via SSH. Single executable including both client and server. Written in Go (Golang). Chisel is mainly useful for passing through firewalls, though it can also be used to provide a secure endpoint into your network. Chisel is very similar to [crowbar](https://github.com/q3k/crowbar) though achieves **much** higher [performance](#performance).

![overview](https://docs.google.com/drawings/d/1p53VWxzGNfy8rjr-mW8pvisJmhkoLl82vAgctO_6f1w/pub?w=960&h=720)

### Install

**Binaries**

[![Releases](https://img.shields.io/github/release/jpillora/chisel.svg)](https://github.com/jpillora/chisel/releases)  [![Releases](https://img.shields.io/github/downloads/jpillora/chisel/total.svg)](https://github.com/jpillora/chisel/releases)

See [the latest release](https://github.com/jpillora/chisel/releases/latest) or download and install it now with `curl https://i.jpillora.com/chisel! | bash`

**Docker**

[![Docker Pulls](https://img.shields.io/docker/pulls/jpillora/chisel.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/jpillora/chisel.svg)][dockerhub]

[dockerhub]: https://hub.docker.com/r/jpillora/chisel/

```sh
docker run --rm -it jpillora/chisel --help
```

**Source**

``` sh
$ go get -v github.com/jpillora/chisel
```

### Features

* Easy to use
* [Performant](#performance)*
* [Encrypted connections](#security) using `crypto/ssh`
* [Authenticated connections](#authentication); authenticated client connections with a users config file, authenticated server connections with fingerprint matching.
* Client auto-reconnects with [exponential backoff](https://github.com/jpillora/backoff)
* Client can create multiple tunnel endpoints over one TCP connection
* Server optionally doubles as a [reverse proxy](http://golang.org/pkg/net/http/httputil/#NewSingleHostReverseProxy)

### Demo

A [demo app](https://chisel-demo.herokuapp.com) on Heroku is running this `chisel server`:

``` sh
$ chisel server --port $PORT --proxy http://example.com
# listens on $PORT, proxy web requests to 'http://example.com'
```

This demo app is also running a [simple file server](https://www.npmjs.com/package/serve) on `:3000`, which is normally inaccessible due to Heroku's firewall. However, if we tunnel in with:

``` sh
$ chisel client https://chisel-demo.herokuapp.com 3000
# connects to 'https://chisel-demo.herokuapp.com',
# tunnels your localhost:3000 to the server's localhost:3000
```

and then visit [localhost:3000](http://localhost:3000/), we should see a directory listing of the demo app's root. Also, if we visit the [demo app](https://chisel-demo.herokuapp.com) in the browser we should hit the server's default proxy and see a copy of [example.com](http://example.com).

### Usage

```
$ chisel --help

	Usage: chisel [command] [--help]

	Version: 0.0.0-src

	Commands:
	  server - runs chisel in server mode
	  client - runs chisel in client mode

	Read more:
	  https://github.com/jpillora/chisel

```

```
$ chisel server --help


  Usage: chisel server [options]

  Options:

    --host, Defines the HTTP listening host – the network interface
    (defaults the environment variable HOST and falls back to 0.0.0.0).

    --port, -p, Defines the HTTP listening port (defaults to the environment
    variable PORT and fallsback to port 8080).

    --key, An optional string to seed the generation of a ECDSA public
    and private key pair. All commications will be secured using this
    key pair. Share the subsequent fingerprint with clients to enable detection
    of man-in-the-middle attacks (defaults to the CHISEL_KEY environment
	variable, otherwise a new key is generate each run).

    --auth, An optional string representing a single user with full
	access, in the form of <user:pass>. This is equivalent to creating an
	authfile with {"<user:pass>": [""]}.

    --authfile, An optional path to a users.json file. This file should
    be an object with users defined like:
      "<user:pass>": ["<addr-regex>","<addr-regex>"]
      when <user> connects, their <pass> will be verified and then
      each of the remote addresses will be compared against the list
      of address regular expressions for a match. Addresses will
      always come in the form "<host/ip>:<port>".

    --proxy, Specifies another HTTP server to proxy requests to when
	chisel receives a normal HTTP request. Useful for hiding chisel in
	plain sight.

    --socks5, Allows client to access the internal SOCKS5 proxy. See
    chisel client --help for more information.

    --pid Generate pid file in current directory

    -v, Enable verbose logging

    --help, This help text

  Version:
    0.0.0-src

  Read more:
    https://github.com/jpillora/chisel

```

```
$ chisel client --help

  Usage: chisel client [options] <server> <remote> [remote] [remote] ...

  <server> is the URL to the chisel server.

  <remote>s are remote connections tunnelled through the server, each of
  which come in the form:

    <local-host>:<local-port>:<remote-host>:<remote-port>

    ■ local-host defaults to 0.0.0.0 (all interfaces).
    ■ local-port defaults to remote-port.
    ■ remote-port is required*.
    ■ remote-host defaults to 0.0.0.0 (server localhost).

    example remotes

      3000
      example.com:3000
      3000:google.com:80
      192.168.0.5:3000:google.com:80
      socks
      5000:socks

    *When the chisel server enables --socks5, remotes can
    specify "socks" in place of remote-host and remote-port.
    The default local host and port for a "socks" remote is
    127.0.0.1:1080. Connections to this remote will terminate
    at the server's internal SOCKS5 proxy.

  Options:

    --fingerprint, A *strongly recommended* fingerprint string
    to perform host-key validation against the server's public key.
    You may provide just a prefix of the key or the entire string.
    Fingerprint mismatches will close the connection.

    --auth, An optional username and password (client authentication)
    in the form: "<user>:<pass>". These credentials are compared to
    the credentials inside the server's --authfile. defaults to the
	AUTH environment variable.

    --keepalive, An optional keepalive interval. Since the underlying
    transport is HTTP, in many instances we'll be traversing through
    proxies, often these proxies will close idle connections. You must
    specify a time with a unit, for example '30s' or '2m'. Defaults
    to '0s' (disabled).

    --proxy, An optional HTTP CONNECT proxy which will be used reach
    the chisel server. Authentication can be specified inside the URL.
	For example, http://admin:password@my-server.com:8081

    --pid Generate pid file in current directory

    -v, Enable verbose logging

    --help, This help text

  Version:
    0.0.0-src

  Read more:
    https://github.com/jpillora/chisel

```

See also [programmatic usage](https://github.com/jpillora/chisel/wiki/Programmatic-Usage).

### Security

Encryption is always enabled. When you start up a chisel server, it will generate an in-memory ECDSA public/private key pair. The public key fingerprint will be displayed as the server starts. Instead of generating a random key, the server may optionally specify a key seed, using the `--key` option, which will be used to seed the key generation. When clients connect, they will also display the server's public key fingerprint. The client can force a particular fingerprint using the `--fingerprint` option. See the `--help` above for more information.

### Authentication

Using the `--authfile` option, the server may optionally provide a `user.json` configuration file to create a list of accepted users. The client then authenticates using the `--auth` option. See [users.json](example/users.json) for an example authentication configuration file. See the `--help` above for more information.

Internally, this is done using the *Password* authentication method provided by SSH. Learn more about `crypto/ssh` here http://blog.gopheracademy.com/go-and-ssh/.

### Performance

With [crowbar](https://github.com/q3k/crowbar), a connection is tunnelled by repeatedly querying the server with updates. This results in a large amount of HTTP and TCP connection overhead. Chisel overcomes this using WebSockets combined with [crypto/ssh](https://golang.org/x/crypto/ssh) to create hundreds of logical connections, resulting in **one** TCP connection per client.

In this simple benchmark, we have:

```
					(direct)
        .--------------->----------------.
       /    chisel         chisel         \
request--->client:2001--->server:2002---->fileserver:3000
       \                                  /
        '--> crowbar:4001--->crowbar:4002'
             client           server
```

Note, we're using an in-memory "file" server on localhost for these tests

*direct*

```
:3000 => 1 bytes in 1.291417ms
:3000 => 10 bytes in 713.525µs
:3000 => 100 bytes in 562.48µs
:3000 => 1000 bytes in 595.445µs
:3000 => 10000 bytes in 1.053298ms
:3000 => 100000 bytes in 741.351µs
:3000 => 1000000 bytes in 1.367143ms
:3000 => 10000000 bytes in 8.601549ms
:3000 => 100000000 bytes in 76.3939ms
```

`chisel`

```
:2001 => 1 bytes in 1.556521ms
:2001 => 10 bytes in 1.310739ms
:2001 => 100 bytes in 1.26706ms
:2001 => 1000 bytes in 1.189441ms
:2001 => 10000 bytes in 1.509267ms
:2001 => 100000 bytes in 2.98981ms
:2001 => 1000000 bytes in 14.737928ms
:2001 => 10000000 bytes in 141.936428ms
:2001 => 100000000 bytes in 1.208960105s
```

~100MB in **~1 second**

`crowbar`

```
:4001 => 1 bytes in 3.335797ms
:4001 => 10 bytes in 1.453007ms
:4001 => 100 bytes in 1.811727ms
:4001 => 1000 bytes in 1.621525ms
:4001 => 10000 bytes in 5.20729ms
:4001 => 100000 bytes in 38.461926ms
:4001 => 1000000 bytes in 358.784864ms
:4001 => 10000000 bytes in 3.603206487s
:4001 => 100000000 bytes in 36.332395213s
```

~100MB in **36 seconds**

See more [test/](test/)

### Known Issues

* WebSockets support is required
	* IaaS providers all will support WebSockets
		* Unless an unsupporting HTTP proxy has been forced in front of you, in which case I'd argue that you've been downgraded to PaaS.
	* PaaS providers vary in their support for WebSockets
		* Heroku has full support
		* Openshift has full support though connections are only accepted on ports 8443 and 8080
		* Google App Engine has **no** support (Track this on [their repo](https://code.google.com/p/googleappengine/issues/detail?id=2535))

### Contributing

* http://golang.org/doc/code.html
* http://golang.org/doc/effective_go.html
* `github.com/jpillora/chisel/share` contains the shared package
* `github.com/jpillora/chisel/server` contains the server package
* `github.com/jpillora/chisel/client` contains the client package

### Changelog

* `1.0.0` - Initial release
* `1.1.0` - Swapped out simple symmetric encryption for ECDSA SSH
* `1.2.0` - Added SOCKS5 (server) and HTTP CONNECT (client) support

### Todo

* Allow clients to act as an indirect tunnel endpoint for other clients
* Better, faster tests
* Expose a stats page for proxy throughput
* Treat client stdin/stdout as a socket

#### MIT License

Copyright © 2017 Jaime Pillora &lt;dev@jpillora.com&gt;

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
