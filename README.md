# chisel

Chisel is TCP proxy tunnelled over HTTP WebSockets. Similar to [crowbar](https://github.com/q3k/crowbar) though achieves **much** higher [performance](#Performance). **Warning** this is beta software.

### Install

Server

```
$ go get -v github.com/jpillora/chisel/chiseld
$ chiseld --help
```

Forwarder

```
$ go get -v github.com/jpillora/chisel/chisel-forward
```

### Demo

A Heroku app is running `chiseld` on the public HTTP port. This app
is also running a file server on 3000 (which is normally inaccessible
due to Heroku's firewall). However, if we tunnel in with:

```
$ chisel-forward --auth foobar https://chisel-demo.herokuapp.com/ 3000
```

Then open [http://localhost:3000/](http://localhost:3000/), we should
see a directory listing of this app's root.

### Usage

Both command-line programs have useful help text, see `chiseld --help` and `chisel-forward --help`.

Eventually, a programmatic API will be documented and available.

### Security

Currently, secure communications are attained by hosting your HTTP server behind a TLS proxy. Thereby upgrading your server to HTTPS. In the future, the server will allow your to pass in TLS credentials and make use of Go's TLS (HTTPS) server.

### Performance

With crowbar, I was getting extremely slow transfer times

```
#tab 1 (local file server)
$ serve -p 4000

#tab 2 (tunnel server)
$ echo -ne "foo:bar" > userfile
$ crowbard -listen="0.0.0.0:8080" -userfile=./userfile

#tab 3 (tunnel client)
$ crowbar-forward -local=0.0.0.0:3000 -server http://localhost:8080 -remote localhost:4000 -username foo -password bar

#tab 4 (transfer test)
$ time curl -s "127.0.0.1:3000/largefile.bin" > /dev/null
       74.74 real         2.37 user         6.74 sys
```

Here, `largefile.bin` (~200MB) is transferred in 1m14s (along with high CPU utilisation).

Enter `chisel`, lets swap in `chiseld` and `chisel-forward`

```
#tab 2 (tunnel server)
$ chiseld --auth foo

#tab 3 (tunnel client)
$ chisel-forward --auth foo http://localhost:8080 3000:4000
2015/02/27 16:13:43 Connected to http://localhost:8080
2015/02/27 16:13:43 Proxy 0.0.0.0:3000 => 0.0.0.0:4000 enabled
```

And now we'll run the test again

```
#tab 4 (transfer test)
$ time curl -s "127.0.0.1:3000/largefile.bin" > /dev/null
       0.60 real         0.05 user         0.14 sys
```

Here, the same file was transferred in 0.6s

### Overview

![overview](https://docs.google.com/drawings/d/1p53VWxzGNfy8rjr-mW8pvisJmhkoLl82vAgctO_6f1w/pub?w=960&h=720)

### Todo

* User file with list of whitelisted remotes
* TLS server configuration
* Expose a stats page for proxy throughput
* Tests along with benchmarks

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
