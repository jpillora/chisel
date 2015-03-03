# chisel

Chisel is an HTTP client and server which acts as a TCP proxy. Chisel useful in situations where you only have access to HTTP, for example – behind a corporate firewall. Chisel is very similar to [crowbar](https://github.com/q3k/crowbar) though achieves **much** higher [performance](#performance). **Warning** This is beta software.

### Install

Server

```
$ go get -v github.com/jpillora/chisel/chiseld
```

Forwarder

```
$ go get -v github.com/jpillora/chisel/chisel-forward
```

### Features

* Simple
* Performant
* Client auto-reconnects with exponential backoff
* Client can have multiple tunnel endpoints over one TCP connection
* Server default proxy

### Demo

A demo app on Heroku is running `chiseld`:

```
$ chiseld --auth foobar --port $PORT --proxy http://example.com
```

This app is also running a file server on 3000 (which is normally inaccessible
due to Heroku's firewall). However, if we tunnel in with:

```
$ chisel-forward --auth foobar https://chisel-demo.herokuapp.com 3000
```

Then open [localhost:3000/](http://localhost:3000/), we should
see a directory listing of this app's root. Also, if we visit
https://chisel-demo.herokuapp.com we should see that the server's
default proxy is pointing at http://example.com.

### Usage

See `chiseld --help` and `chisel-forward --help`.

Eventually, a programmatic API will be documented and available, if you're keen see the `main.go` files in each sub-package.

### Security

Currently, you can only secure your traffic by using HTTPS, which can only be done by hosting your HTTP server behind a TLS terminating proxy (like Heroku's router). In the future, the server will allow your to pass in TLS credentials and make use of Go's TLS (HTTPS) server.

### Performance

With crowbar, a connection is tunnelled by repeatedly querying the server with updates. This results in a large amount of HTTP and TCP connection overhead. Chisel overcomes this using WebSockets combined with [Yamux](https://github.com/hashicorp/yamux) to create SDPY/HTTP2 like logical connections, therefore, each client will only need 1 TCP connection.

In this test, we have:

```
curl -> http tunnel client -> http tunnel server -> file server
```

*Tab 1 (local file server)*

```
$ npm i -g serve
$ serve -p 4000
```

*Tab 2 (tunnel server)*

```
$ echo -ne "foo:bar" > userfile
$ crowbard -listen="0.0.0.0:8080" -userfile=./userfile
```

*Tab 3 (tunnel client)*

```
$ crowbar-forward -local=0.0.0.0:3000 -server http://localhost:8080 -remote localhost:4000 -username foo -password bar
```

*Tab 4 (transfer test)*

```
$ time curl -s "127.0.0.1:3000/largefile.bin" > /dev/null
       74.74 real         2.37 user         6.74 sys
```

Here, we see `largefile.bin` (~200MB) is transferred in **1m14s** (along with high CPU utilisation).

Enter `chisel`, lets swap in `chiseld` and `chisel-forward`

*Tab 2 (tunnel server)*

```
$ chiseld --auth foo
```

*Tab 3 (tunnel client)*

```
$ chisel-forward --auth foo localhost:8080 3000:4000
2015/02/27 16:13:43 Connected to http://localhost:8080
2015/02/27 16:13:43 Proxy 0.0.0.0:3000 => 0.0.0.0:4000 enabled
```

And now we'll run the test again

```
$ time curl -s "127.0.0.1:3000/largefile.bin" > /dev/null
       0.60 real         0.05 user         0.14 sys
```

Here, the same file was transferred in **0.6s**

### Overview

![overview](https://docs.google.com/drawings/d/1p53VWxzGNfy8rjr-mW8pvisJmhkoLl82vAgctO_6f1w/pub?w=960&h=720)

### Contributing

* http://golang.org/doc/code.html
* http://golang.org/doc/effective_go.html
* `github.com/jpillora/chisel` contains the shared package
* `github.com/jpillora/chisel/chiseld` contains the server package
* `github.com/jpillora/chisel/chisel-forward` contains the client package

### Todo

* Add tests (Bonus: Add benchmarks)
* User file with list of whitelisted remotes
* Pass in TLS server configuration
* Encrypt data with `auth` as the symmetric key
* Expose a stats page for proxy throughput
* Configurable connection retry times

#### MIT License

Copyright © 2014 Jaime Pillora &lt;dev@jpillora.com&gt;

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
