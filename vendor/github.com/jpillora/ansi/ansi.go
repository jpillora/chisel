//Implements the ANSI VT100 control set.
//Please refer to http://www.termsys.demon.co.uk/vtansi.htm
package ansi

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

//Ansi represents a wrapped io.ReadWriter.
//It will read the stream, parse and remove ANSI report codes
//and place them on the Reports queue.
type Ansi struct {
	rw      io.ReadWriter
	rerr    error
	rbuff   chan []byte
	Reports chan *Report
}

//Wrap an io.ReadWriter (like a net.Conn) to
//easily read and write control codes
func Wrap(rw io.ReadWriter) *Ansi {
	a := &Ansi{}
	a.rw = rw
	a.rbuff = make(chan []byte)
	a.Reports = make(chan *Report)
	go a.read()
	return a
}

var reportCode = regexp.MustCompile(`\[([^a-zA-Z]*)(0c|0n|3n|R)`)

//reads the underlying ReadWriter for real,
//extracts the ansi codes, places the rest
//in the read buffer
func (a *Ansi) read() {
	buff := make([]byte, 0xffff)
	for {
		n, err := a.rw.Read(buff)
		if err != nil {
			a.rerr = err
			close(a.rbuff)
			break
		}

		var src = buff[:n]
		var dst []byte

		//contain ansi codes?
		m := reportCode.FindAllStringSubmatchIndex(string(src), -1)

		if len(m) == 0 {
			dst = make([]byte, n)
			copy(dst, src)
		} else {
			for _, i := range m {
				//slice off ansi code body and trailing char
				a.parse(string(src[i[2]:i[3]]), string(src[i[4]:i[5]]))
				//add surrounding bits to dst buffer
				dst = append(dst, src[:i[0]]...)
				dst = append(dst, src[i[1]:]...)
			}
			if len(dst) == 0 {
				continue
			}
		}

		a.rbuff <- dst
	}
}

// Report Device Code	<ESC>[{code}0c
// Report Device OK	<ESC>[0n
// Report Device Failure	<ESC>[3n
// Report Cursor Position	<ESC>[{ROW};{COLUMN}R
func (a *Ansi) parse(body, char string) {
	r := &Report{}
	switch char {
	case "0c":
		r.Type = Code
		r.Code, _ = strconv.Atoi(body)
	case "0n":
		r.Type = OK
	case "3n":
		r.Type = Failure
	case "R":
		r.Type = Position
		pair := strings.Split(body, ";")
		r.Pos.Col, _ = strconv.Atoi(pair[1])
		r.Pos.Row, _ = strconv.Atoi(pair[0])
	default:
		return
	}
	// fmt.Printf("parsed report: %+v", r)
	a.Reports <- r
}

//Reads the underlying ReadWriter
func (a *Ansi) Read(dest []byte) (n int, err error) {
	//It doesn't really read the underlying ReadWriter :)
	if a.rerr != nil {
		return 0, a.rerr
	}
	src, open := <-a.rbuff
	if !open {
		return 0, a.rerr
	}
	return copy(dest, src), nil
}

//Writes the underlying ReadWriter
func (a *Ansi) Write(p []byte) (n int, err error) {
	return a.rw.Write(p)
}

//Closes the underlying ReadWriter
func (a *Ansi) Close() error {
	c, ok := a.rw.(io.Closer)
	if !ok {
		return errors.New("Provided ReadWriter is not a Closer")
	}
	return c.Close()
}

//==============================

type ReportType int

const (
	Code ReportType = iota
	OK
	Failure
	Position
)

type Report struct {
	Type ReportType
	Code int
	Pos  struct {
		Row, Col int
	}
}

//==============================

const Esc = byte(27)

var QueryCode = []byte{Esc, '[', 'c'}
var QueryDeviceStatus = []byte{Esc, '[', '5', 'n'}
var QueryCursorPosition = []byte{Esc, '[', '6', 'n'}

func (a *Ansi) QueryCursorPosition() {
	a.Write(QueryCursorPosition)
}

var ResetDevice = []byte{Esc, 'c'}

var EnableLineWrap = []byte{Esc, '[', '7', 'h'}

func (a *Ansi) EnableLineWrap() {
	a.Write(DisableLineWrap)
}

var DisableLineWrap = []byte{Esc, '[', '7', 'l'}

func (a *Ansi) DisableLineWrap() {
	a.Write(DisableLineWrap)
}

var FontSetG0 = []byte{Esc, '('}
var FontSetG1 = []byte{Esc, ')'}

// Cursor Home 		<ESC>[{ROW};{COLUMN}H
// Cursor Up		<ESC>[{COUNT}A
// Cursor Down		<ESC>[{COUNT}B
// Cursor Forward		<ESC>[{COUNT}C
// Cursor Backward		<ESC>[{COUNT}D
// Force Cursor Position	<ESC>[{ROW};{COLUMN}f
func Goto(r, c uint16) []byte {
	rb := []byte(strconv.Itoa(int(r)))
	cb := []byte(strconv.Itoa(int(c)))
	b := append([]byte{Esc, '['}, rb...)
	b = append(b, ';')
	b = append(b, cb...)
	b = append(b, 'f')
	return b
}

func (a *Ansi) Goto(r, c uint16) {
	a.Write(Goto(r, c))
}

var SaveCursor = []byte{Esc, '[', 's'}
var UnsaveCursor = []byte{Esc, '[', 'u'}
var SaveAttrCursor = []byte{Esc, '7'}
var RestoreAttrCursor = []byte{Esc, '8'}
var CursorHide = []byte{Esc, '[', '?', '2', '5', 'l'}

func (a *Ansi) CursorHide() {
	a.Write(CursorHide)
}

var CursorShow = []byte{Esc, '[', '?', '2', '5', 'h'}

func (a *Ansi) CursorShow() {
	a.Write(CursorShow)
}

var ScrollScreen = []byte{Esc, '[', 'r'}
var ScrollDown = []byte{Esc, 'D'}
var ScrollUp = []byte{Esc, 'M'}

func Scroll(start, end uint16) []byte {
	return []byte(string(Esc) + fmt.Sprintf("[%d;%dr", start, end))
}

// Tab Control
var SetTab = []byte{Esc, 'H'}
var ClearTab = []byte{Esc, '[', 'g'}
var ClearAllTabs = []byte{Esc, '[', '3', 'g'}
var EraseEndLine = []byte{Esc, '[', 'K'}
var EraseStartLine = []byte{Esc, '[', '1', 'K'}
var EraseLine = []byte{Esc, '[', '2', 'K'}
var EraseDown = []byte{Esc, '[', 'J'}
var EraseUp = []byte{Esc, '[', '1', 'J'}
var EraseScreen = []byte{Esc, '[', '2', 'J'}

func (a *Ansi) EraseScreen() {
	a.Write(EraseScreen)
}

// Printing
var PrintScreen = []byte{Esc, '[', 'i'}
var PrintLine = []byte{Esc, '[', '1', 'i'}
var StopPrintLog = []byte{Esc, '[', '4', 'i'}
var StartPrintLog = []byte{Esc, '[', '5', 'i'}

// Set Key Definition	<ESC>[{key};"{ascii}"p

// Sets multiple display attribute settings. The following lists standard attributes:
type Attribute string

const (
	//formatinn
	Reset      Attribute = "0"
	Bright     Attribute = "1"
	Dim        Attribute = "2"
	Italic     Attribute = "3"
	Underscore Attribute = "4"
	Blink      Attribute = "5"
	Reverse    Attribute = "7"
	Hidden     Attribute = "8"
	//foreground colors
	Black   Attribute = "30"
	Red     Attribute = "31"
	Green   Attribute = "32"
	Yellow  Attribute = "33"
	Blue    Attribute = "34"
	Magenta Attribute = "35"
	Cyan    Attribute = "36"
	White   Attribute = "37"
	//background colors
	BlackBG   Attribute = "40"
	RedBG     Attribute = "41"
	GreenBG   Attribute = "42"
	YellowBG  Attribute = "43"
	BlueBG    Attribute = "44"
	MagentaBG Attribute = "45"
	CyanBG    Attribute = "46"
	WhiteBG   Attribute = "47"
)

//Set attributes
func Set(attrs ...Attribute) []byte {
	s := make([]string, len(attrs))
	for i, a := range attrs {
		s[i] = string(a)
	}
	b := []byte(strings.Join(s, ";"))
	b = append(b, 'm')
	return append([]byte{Esc, '['}, b...)
}

// Set Attribute Mode	<ESC>[{attr1};...;{attrn}m
func (a *Ansi) Set(attrs ...Attribute) {
	a.Write(Set(attrs...))
}

var (
	ResetBytes      = Set(Reset)
	BrightBytes     = Set(Bright)
	DimBytes        = Set(Dim)
	ItalicBytes     = Set(Italic)
	UnderscoreBytes = Set(Underscore)
	BlinkBytes      = Set(Blink)
	ReverseBytes    = Set(Reverse)
	HiddenBytes     = Set(Hidden)
	//foreground colors
	BlackBytes   = Set(Black)
	RedBytes     = Set(Red)
	GreenBytes   = Set(Green)
	YellowBytes  = Set(Yellow)
	BlueBytes    = Set(Blue)
	MagentaBytes = Set(Magenta)
	CyanBytes    = Set(Cyan)
	WhiteBytes   = Set(White)
	//background colors
	BlackBGBytes   = Set(BlackBG)
	RedBGBytes     = Set(RedBG)
	GreenBGBytes   = Set(GreenBG)
	YellowBGBytes  = Set(YellowBG)
	BlueBGBytes    = Set(BlueBG)
	MagentaBGBytes = Set(MagentaBG)
	CyanBGBytes    = Set(CyanBG)
	WhiteBGBytes   = Set(WhiteBG)
)
