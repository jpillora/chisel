## ansi

Implements the ANSI VT100 control set.
Please refer to http://www.termsys.demon.co.uk/vtansi.htm

### Install

```
go get github.com/jpillora/ansi
```

### Usage

Get ANSI control code bytes:

``` go
ansi.Goto(2,4)
ansi.Set(ansi.Green, ansi.BlueBG)
```

Wrap an `io.ReadWriteCloser`:

``` go

a := ansi.Wrap(tcpConn)

//Read, Write, Close as normal
a.Read()
a.Write()
a.Close()

//Shorthand for a.Write(ansi.Set(..))
a.Set(ansi.Green, ansi.BlueBG)

//Send query
a.QueryCursorPosition()
//Await report
report := <- a.Reports
report.Type//=> ansi.Position
report.Pos.Row
report.Pos.Col
```

*Wrapped connections will intercept and remove ANSI report codes from `a.Read()`*

### API

http://godoc.org/github.com/jpillora/ansi

#### MIT License

Copyright © 2014 &lt;dev@jpillora.com&gt;

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