# requestlog

Simple request logging in Go (Golang)

### Install

``` sh
$ go get -v github.com/jpillora/requestlog
```

### Usage

``` go
h := http.Handler(...)
h = requestlog.Wrap(h)
```

And you'll see something like:

```
2015/07/08 21:43:03.063 GET /a 200 90ms 912B
2015/07/08 21:43:03.148 GET /b 200 80ms 320B
2015/07/08 21:43:03.242 GET /c 200 90ms 116B
2015/07/08 21:43:03.320 GET /d 200 73ms 2B
2015/07/08 21:43:03.399 GET /e 200 74ms 2B
2015/07/08 21:43:03.476 GET /f 200 72ms 2B
2015/07/08 21:43:03.558 GET /g 200 77ms 2B
```

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