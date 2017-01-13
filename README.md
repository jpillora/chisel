# chisel

Chisel is a fast TCP tunnel, transported over HTTP. Single executable including both client and server. Written in Go (Golang). Chisel is mainly useful for passing through firewalls, though it can also be used to provide a secure endpoint into your network. Chisel is very similar to [crowbar](https://github.com/q3k/crowbar) though achieves **much** higher [performance](#performance). **Warning** Chisel is currently beta software.

![overview](https://docs.google.com/drawings/d/1p53VWxzGNfy8rjr-mW8pvisJmhkoLl82vAgctO_6f1w/pub?w=960&h=720)

### Install

**Binaries**

See [the latest release](https://github.com/jpillora/chisel/releases/latest)

**Docker**

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

<tmpl,code: chisel --help>
```

	Usage: chisel [command] [--help]

	Version: 0.0.0-src

	Commands:
	  server - runs chisel in server mode
	  client - runs chisel in client mode

	Read more:
	  https://github.com/jpillora/chisel

```
</tmpl>

`chisel server --help`

<tmpl,code: chisel server --help>
```

	Usage: chisel server [options]

	Options:

	  --host, Defines the HTTP listening host – the network interface
	  (defaults to 0.0.0.0).

	  --port, Defines the HTTP listening port (defaults to 8080).

	  --key, An optional string to seed the generation of a ECDSA public
	  and private key pair. All communications will be secured using this
	  key pair. Share this fingerprint with clients to enable detection
	  of man-in-the-middle attacks.

	  --authfile, An optional path to a users.json file. This file should
	  be an object with users defined like:
	    "<user:pass>": ["<addr-regex>","<addr-regex>"]
	    when <user> connects, their <pass> will be verified and then
	    each of the remote addresses will be compared against the list
	    of address regular expressions for a match. Addresses will
	    always come in the form "<host/ip>:<port>".

	  --proxy, Specifies the default proxy target to use when chisel
	  receives a normal HTTP request.

	  -v, Enable verbose logging

	  --help, This help text

	Read more:
	  https://github.com/jpillora/chisel

```
</tmpl>

`chisel client --help`

<tmpl,code: chisel client --help>
```

	Usage: chisel client [options] <server> <remote> [remote] [remote] ...

	server is the URL to the chisel server.

	remotes are remote connections tunnelled through the server, each of
	which come in the form:

		<local-host>:<local-port>:<remote-host>:<remote-port>

		* remote-port is required.
		* local-port defaults to remote-port.
		* local-host defaults to 0.0.0.0 (all interfaces).
		* remote-host defaults to 0.0.0.0 (server localhost).

		example remotes

			3000
			example.com:3000
			3000:google.com:80
			192.168.0.5:3000:google.com:80

	Options:

	  --fingerprint, An optional fingerprint (server authentication)
	  string to compare against the server's public key. You may provide
	  just a prefix of the key or the entire string. Fingerprint 
	  mismatches will close the connection.

	  --auth, An optional username and password (client authentication)
	  in the form: "<user>:<pass>". These credentials are compared to
	  the credentials inside the server's --authfile.

	  --keepalive, An optional keepalive interval. Since the underlying
	  transport is HTTP, in many instances we'll be traversing through
	  proxies, often these proxies will close idle connections. You must
	  specify a time with a unit, for example '30s' or '2m'. Defaults
	  to '0s' (disabled).

	  -v, Enable verbose logging

	  --help, This help text

	Read more:
	  https://github.com/jpillora/chisel

```
</tmpl>

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
:3000 => 1 bytes in 1.440608ms
:3000 => 10 bytes in 658.833µs
:3000 => 100 bytes in 669.6µs
:3000 => 1000 bytes in 570.242µs
:3000 => 10000 bytes in 655.795µs
:3000 => 100000 bytes in 693.761µs
:3000 => 1000000 bytes in 2.156777ms
:3000 => 10000000 bytes in 18.562896ms
:3000 => 100000000 bytes in 146.355886ms
```

`chisel`

```
:2001 => 1 bytes in 1.393731ms
:2001 => 10 bytes in 1.002992ms
:2001 => 100 bytes in 1.082757ms
:2001 => 1000 bytes in 1.096081ms
:2001 => 10000 bytes in 1.215036ms
:2001 => 100000 bytes in 2.09334ms
:2001 => 1000000 bytes in 9.136138ms
:2001 => 10000000 bytes in 84.170904ms
:2001 => 100000000 bytes in 796.713039ms
```

~100MB in **0.8 seconds**

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
		* Google App Engine has **no** support

### Contributing

* http://golang.org/doc/code.html
* http://golang.org/doc/effective_go.html
* `github.com/jpillora/chisel/share` contains the shared package
* `github.com/jpillora/chisel/server` contains the server package
* `github.com/jpillora/chisel/client` contains the client package

### Changelog

* `1.0.0` - Init
* `1.1.0` - Swapped out simple symmetric encryption for ECDSA SSH

### Todo

* Better, faster tests
* Expose a stats page for proxy throughput
* Treat client stdin/stdout as a socket
* Allow clients to act as an indirect tunnel endpoint for other clients
* Keep local connections open and buffer between remote retries

#### MIT License

Copyright © 2015 Jaime Pillora &lt;dev@jpillora.com&gt;

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
