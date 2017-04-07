# sizestr

Convert `231938` into `232KB`

[![GoDoc](https://godoc.org/github.com/jpillora/sizestr?status.svg)](https://godoc.org/github.com/jpillora/sizestr) [![Circle CI](https://circleci.com/gh/jpillora/sizestr.svg?style=svg)](https://circleci.com/gh/jpillora/sizestr)

### Usage

```
go get github.com/jpillora/sizestr
```

``` go
//use directly
sizestr.ToString(231938) //"232KB"
sizestr.Parse("232KB") //232000
sizestr.Parse("232KiB") //237568
//use with pkg/flag
var b sizestr.Bytes
flag.Var(&b, "size", "the size of my file")
```

#### Significant Figures

Default is `3`. Set via:

``` go
sizestr.ToStringSig(231938, 4) //"231.9KB"
```

#### Scale

Default is `1000`, standard units dictates that Kilo is 1000, not 1024. Also, see this [blog post](https://blogs.gnome.org/cneumair/2008/09/30/1-kb-1024-bytes-no-1-kb-1000-bytes/). Though can set via:

``` go
sizestr.ToStringSig(231938, 4, 1024) //"226.5KB"
```

---

#### `ToPrecision`

This library also contains a Go implementation of JavaScript's `Number.prototype.toPrecision`

``` go
sizestr.ToPrecision(123.456, 4) //123.5
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
