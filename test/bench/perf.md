
### Performance

With [crowbar](https://github.com/q3k/crowbar), a connection is tunneled by repeatedly querying the server with updates. This results in a large amount of HTTP and TCP connection overhead. Chisel overcomes this using WebSockets combined with [crypto/ssh](https://golang.org/x/crypto/ssh) to create hundreds of logical connections, resulting in **one** TCP connection per client.

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

_direct_

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
:2001 => 1 bytes in 1.351976ms
:2001 => 10 bytes in 1.106086ms
:2001 => 100 bytes in 1.005729ms
:2001 => 1000 bytes in 1.254396ms
:2001 => 10000 bytes in 1.139777ms
:2001 => 100000 bytes in 2.35437ms
:2001 => 1000000 bytes in 11.502673ms
:2001 => 10000000 bytes in 123.130246ms
:2001 => 100000000 bytes in 966.48636ms
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

See `test/bench/main.go`