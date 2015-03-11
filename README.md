# chisel

Chisel is an HTTP client and server which acts as a TCP proxy. Chisel useful in situations where you only have access to HTTP, for example – behind a corporate firewall. Chisel is very similar to [crowbar](https://github.com/q3k/crowbar) though achieves **much** higher [performance](#performance). **Warning** This is beta software.

![overview](https://docs.google.com/drawings/d/1p53VWxzGNfy8rjr-mW8pvisJmhkoLl82vAgctO_6f1w/pub?w=960&h=720)

### Install

**Binaries**

See [Releases](https://github.com/jpillora/chisel/releases)

**Source**

``` sh
# chisel server
$ go get -v github.com/jpillora/chisel/chiseld
# chisel client
$ go get -v github.com/jpillora/chisel/chisel-forward
```

### Features

* Easy to use
* Performant
* Client auto-reconnects with [exponential backoff](https://github.com/jpillora/backoff)
* Client can create multiple tunnel endpoints over one TCP connection
* Server optionally doubles as a [reverse proxy](http://golang.org/pkg/net/http/httputil/#NewSingleHostReverseProxy)

### Demo

A [demo app](https://chisel-demo.herokuapp.com) on Heroku is running this `chiseld` server:

``` sh
$ chiseld --auth foobar --port $PORT --proxy http://example.com
# listens on $PORT, requires password 'foobar', proxy web requests to 'http://example.com'
```

This demo app is also running a file server on 0.0.0.0:3000 (which is normally inaccessible
due to Heroku's firewall). However, if we tunnel in with:

``` sh
$ chisel-forward --auth foobar https://chisel-demo.herokuapp.com 3000
# connects to 'https://chisel-demo.herokuapp.com', using password 'foobar',
# tunnels your localhost:3000 to the server's localhost:3000
```

Then open [localhost:3000/](http://localhost:3000/), we should
see a directory listing of the demo app's root. Also, if we visit
[the demo](https://chisel-demo.herokuapp.com) in the browser we should see that the server's
default proxy is pointing at [example.com](http://example.com).

### Usage

```
$ chiseld --help

	Usage: chiseld [options]

	Options:

	--host, Defines the HTTP listening host – the network interface
	(defaults to 0.0.0.0). You may also set the HOST environment
	variable.

	--port, Defines the HTTP listening port (defaults to 8080). You
	may also set the PORT environment variable.

	--auth, Specifies the exact authentication string the client must
	provide to attain access. You may also set the AUTH environment
	variable.

	--proxy, Specifies the default proxy target to use when chiseld
	receives a normal HTTP request.

	-v, Enable verbose logging

	--version, Display version

	Read more:
	https://github.com/jpillora/chisel
```

```
$ chisel-forward --help

	Usage: chisel-forward [options] server remote [remote] [remote] ...

	server is the URL to the chiseld server.

	remotes are remote connections tunneled through the server, each of
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

	--auth AUTH, Specifies the optional authentication string used by
	the server.

	-v, Enable verbose logging

	--version, Display version

	Read more:
	https://github.com/jpillora/chisel
```

See also: [programmatic API](https://github.com/jpillora/chisel/wiki/Programmatic-Usage).

### Security

Currently, you can secure your traffic by using HTTPS, which can only be done by hosting your HTTP server behind a TLS terminating proxy (like Heroku's router). In the future, the server will allow your to pass in TLS credentials and make use of Go's TLS (HTTPS) server.

### Performance

With [crowbar](https://github.com/q3k/crowbar), a connection is tunnelled by repeatedly querying the server with updates. This results in a large amount of HTTP and TCP connection overhead. Chisel overcomes this using WebSockets combined with [Yamux](https://github.com/hashicorp/yamux) to create hundreds of SDPY/HTTP2 like logical connections, resulting in **one** TCP connection per client.

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
:3000 => 1 bytes in 1.008883ms
:3000 => 10 bytes in 543.198µs
:3000 => 100 bytes in 675.957µs
:3000 => 1000 bytes in 584.13µs
:3000 => 10000 bytes in 580.56µs
:3000 => 100000 bytes in 743.902µs
:3000 => 1000000 bytes in 1.962673ms
:3000 => 10000000 bytes in 19.192986ms
:3000 => 100000000 bytes in 158.428239ms
```

`chisel`

```
:2001 => 1 bytes in 1.334661ms
:2001 => 10 bytes in 807.797µs
:2001 => 100 bytes in 763.728µs
:2001 => 1000 bytes in 1.029811ms
:2001 => 10000 bytes in 840.247µs
:2001 => 100000 bytes in 1.647748ms
:2001 => 1000000 bytes in 3.495904ms
:2001 => 10000000 bytes in 22.298904ms
:2001 => 100000000 bytes in 255.410448ms
```

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

See [test/](test/)

### Known Issues

* **WebSockets support is required**
	* IaaS providers all will support WebSockets
		* Unless an unsupporting HTTP proxy has been forced in front of you, in which case I'd argue that you've been downgraded to PaaS.
	* PaaS providers vary in their support for WebSockets
		* Heroku has full support
		* Openshift has full support though connections are only accepted on ports 8443 and 8080
		* Google App Engine has **no** support


### Contributing

* http://golang.org/doc/code.html
* http://golang.org/doc/effective_go.html
* `github.com/jpillora/chisel` contains the shared package
* `github.com/jpillora/chisel/chiseld` contains the server package
* `github.com/jpillora/chisel/chisel-forward` contains the client package

### Todo

* Users file with white-listed remotes
* Pass in TLS server configuration
* Encrypt data with `auth` as the symmetric key
* Expose a stats page for proxy throughput
* Configurable connection retry times
* Treat forwarder stdin/stdout as a socket

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
