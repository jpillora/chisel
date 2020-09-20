package settings

import (
	"reflect"
	"testing"
)

func TestRemoteDecode(t *testing.T) {
	//test table
	for i, test := range []struct {
		Input   string
		Output  Remote
		Encoded string
	}{
		{
			"3000",
			Remote{
				LocalPort:  "3000",
				RemoteHost: "127.0.0.1",
				RemotePort: "3000",
			},
			"0.0.0.0:3000:127.0.0.1:3000",
		},
		{
			"google.com:80",
			Remote{
				LocalPort:  "80",
				RemoteHost: "google.com",
				RemotePort: "80",
			},
			"0.0.0.0:80:google.com:80",
		},
		{
			"R:google.com:80",
			Remote{
				LocalPort:  "80",
				RemoteHost: "google.com",
				RemotePort: "80",
				Reverse:    true,
			},
			"R:0.0.0.0:80:google.com:80",
		},
		{
			"示例網站.com:80",
			Remote{
				LocalPort:  "80",
				RemoteHost: "示例網站.com",
				RemotePort: "80",
			},
			"0.0.0.0:80:示例網站.com:80",
		},
		{
			"socks",
			Remote{
				LocalHost: "127.0.0.1",
				LocalPort: "1080",
				Socks:     true,
			},
			"127.0.0.1:1080:socks",
		},
		{
			"127.0.0.1:1081:socks",
			Remote{
				LocalHost: "127.0.0.1",
				LocalPort: "1081",
				Socks:     true,
			},
			"127.0.0.1:1081:socks",
		},
		{
			"1.1.1.1:53/udp",
			Remote{
				LocalPort:   "53",
				LocalProto:  "udp",
				RemoteHost:  "1.1.1.1",
				RemotePort:  "53",
				RemoteProto: "udp",
			},
			"0.0.0.0:53:1.1.1.1:53/udp",
		},
		{
			"localhost:5353:1.1.1.1:53/udp",
			Remote{
				LocalHost:   "localhost",
				LocalPort:   "5353",
				LocalProto:  "udp",
				RemoteHost:  "1.1.1.1",
				RemotePort:  "53",
				RemoteProto: "udp",
			},
			"localhost:5353:1.1.1.1:53/udp",
		},
		{
			"[::1]:8080:google.com:80",
			Remote{
				LocalHost:  "[::1]",
				LocalPort:  "8080",
				RemoteHost: "google.com",
				RemotePort: "80",
			},
			"[::1]:8080:google.com:80",
		},
		{
			"R:[::]:3000:[::1]:3000",
			Remote{
				LocalHost:  "[::]",
				LocalPort:  "3000",
				RemoteHost: "[::1]",
				RemotePort: "3000",
				Reverse:    true,
			},
			"R:[::]:3000:[::1]:3000",
		},
	} {
		//expected defaults
		expected := test.Output
		if expected.LocalHost == "" {
			expected.LocalHost = "0.0.0.0"
		}
		if expected.RemoteProto == "" {
			expected.RemoteProto = "tcp"
		}
		if expected.LocalProto == "" {
			expected.LocalProto = "tcp"
		}
		//compare
		got, err := DecodeRemote(test.Input)
		if err != nil {
			t.Fatalf("decode #%d '%s' failed: %s", i+1, test.Input, err)
		}
		if !reflect.DeepEqual(got, &expected) {
			t.Fatalf("decode #%d '%s' expected\n  %#v\ngot\n  %#v", i+1, test.Input, expected, got)
		}
		if e := got.Encode(); test.Encoded != e {
			t.Fatalf("encode #%d '%s' expected\n  %#v\ngot\n  %#v", i+1, test.Input, test.Encoded, e)
		}
	}
}
