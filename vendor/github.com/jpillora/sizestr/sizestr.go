package sizestr

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

//String representations of each scale
var scaleStrings = []string{"B", "KB", "MB", "GB", "TB", "PB", "XB"}
var parseRegexp = regexp.MustCompile(
	//     byte value[1]  scales[4]    1024[5]? B
	`(?i)\b(\d+(\.(\d+))?)(k|m|g|t|p|x)?(i?)(b?)\b`,
)
var lowerCase = false

//Default bytes per kilobyte
var defaultBytesPerKB = float64(1000)
var defaultBytesPerKiB = float64(1024)

//Default number of Significant Figures
var defaultSigFigures = float64(3) //must 10^SigFigures >= Scale

//ToggleCase changes the case of the scale strings ("MB" -> "mb")
func ToggleCase() {
	lowerCase = !lowerCase
}

func UpperCase() {
	lowerCase = false
}

func LowerCase() {
	lowerCase = false
}

//Converts a byte count into a byte string
func ToString(n int64) string {
	return ToStringSigBytesPerKB(n, defaultSigFigures, defaultBytesPerKB)
}

//ParseScale a string into a byte count with a specific scale
func MustParse(s string) int64 {
	i, err := ParseBytesPerKB(s, 0 /*autodetect*/)
	if err != nil {
		panic(err)
	}
	return i
}

//ParseScale a string into a byte count with a specific scale
func Parse(s string) (int64, error) {
	return ParseBytesPerKB(s, 0 /*autodetect*/)
}

//ParseScale a string into a byte count with a specific scale (defaults to 1000)
func ParseBytesPerKB(s string, bytesPerKB int64) (int64, error) {
	//0 doesn't need a scale
	if s == "0" {
		return 0, nil
	}
	m := parseRegexp.FindStringSubmatch(s)
	if len(m) == 0 {
		return 0, errors.New("parse failed")
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, errors.New("parse float error")
	}
	var bytesPer float64
	if bytesPerKB > 0 {
		bytesPer = float64(bytesPerKB)
	} else {
		if strings.ToLower(m[5]) == "i" {
			bytesPer = float64(defaultBytesPerKiB)
		} else {
			bytesPer = float64(defaultBytesPerKB)
		}
	}
	if strings.ToLower(m[6]) == "b" {
		scale := strings.ToUpper(m[4] + "b")
		for _, s := range scaleStrings {
			if scale == s {
				break
			}
			v *= bytesPer
		}
	}
	i := int64(v)
	if i < 0 {
		return 0, errors.New("int64 overflow")
	}
	return i, nil
}

//Converts a byte count into a byte string
func ToStringSig(n int64, sig float64) string {
	return ToStringSigBytesPerKB(n, sig, defaultBytesPerKB)
}

//Converts a byte count into a byte string
func ToStringSigBytesPerKB(n int64, sig, bytesPerKB float64) string {
	var f = float64(n)
	var i int
	for i, _ = range scaleStrings {
		if f < bytesPerKB {
			break
		}
		f = f / bytesPerKB
	}
	f = ToPrecision(f, sig)
	if f == bytesPerKB {
		return strconv.FormatFloat(f/bytesPerKB, 'f', 0, 64) + toCase(scaleStrings[i+1])
	}
	return strconv.FormatFloat(f, 'f', -1, 64) + toCase(scaleStrings[i])
}

var log10 = math.Log(10)

//A Go implementation of JavaScript's Math.toPrecision
func ToPrecision(n, p float64) float64 {
	//credits http://stackoverflow.com/a/12055126/977939
	if n == 0 {
		return 0
	}
	e := math.Floor(math.Log10(math.Abs(n)))
	f := round(math.Exp(math.Abs(e-p+1) * log10))
	if e-p+1 < 0 {
		return round(n*f) / f
	}
	return round(n/f) * f
}

func round(n float64) float64 {
	return math.Floor(n + 0.5)
}

func toCase(s string) string {
	if lowerCase {
		return strings.ToLower(s)
	}
	return s
}
