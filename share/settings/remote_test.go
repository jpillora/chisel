package settings

import (
	"reflect"
	"strings"
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
			//uppercase suffix parses to lowercase proto
			"1.1.1.1:53/UDP",
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

func TestUDPSocksRemote(t *testing.T) {
	for _, test := range []struct {
		input, host, port string
		reverse           bool
	}{
		{"udpsocks", "127.0.0.1", "1080", false},
		{"5000:udpsocks", "127.0.0.1", "5000", false},
		{"0.0.0.0:5000:udpsocks", "0.0.0.0", "5000", false},
		{"localhost:5000:udpsocks", "localhost", "5000", false},
		{"[::1]:5000:udpsocks", "[::1]", "5000", false},
		{"R:udpsocks", "127.0.0.1", "1080", true},
		{"R:5000:udpsocks", "127.0.0.1", "5000", true},
		{"R:[::]:5000:udpsocks", "[::]", "5000", true},
	} {
		t.Run(test.input, func(t *testing.T) {
			r, err := DecodeRemote(test.input)
			if err != nil {
				t.Fatal(err)
			}
			want := &Remote{LocalHost: test.host, LocalPort: test.port, LocalProto: "tcp", RemoteProto: "tcp", Socks: true, UDPSocks: true, Reverse: test.reverse}
			if !reflect.DeepEqual(r, want) {
				t.Fatalf("decoded %#v, want %#v", r, want)
			}
			encoded := test.host + ":" + test.port + ":udpsocks"
			permission := "socks"
			if test.reverse {
				encoded = "R:" + encoded
				permission = "R:" + test.host + ":" + test.port
			}
			if r.Encode() != encoded || r.Remote() != "udpsocks" || !strings.HasSuffix(r.String(), "=>udpsocks") || r.UserAddr() != permission {
				t.Fatalf("encode=%q remote=%q string=%q permission=%q", r.Encode(), r.Remote(), r.String(), r.UserAddr())
			}
			decoded, err := DecodeRemote(r.Encode())
			if err != nil || !reflect.DeepEqual(decoded, r) {
				t.Fatalf("remote round trip: %#v, %v", decoded, err)
			}
			config := Config{Version: "test", Remotes: Remotes{r}}
			got, err := DecodeConfig(EncodeConfig(config))
			if err != nil || !reflect.DeepEqual(got, &config) {
				t.Fatalf("configuration round trip: %#v, %v", got, err)
			}
		})
	}
}

func TestUDPSocksInvalidRemote(t *testing.T) {
	for _, input := range []string{
		"0:udpsocks", "127.0.0.1:0:udpsocks", "R:0:udpsocks",
		"-1:udpsocks", "65536:udpsocks", "1080:1081:udpsocks",
		"localhost:localhost:udpsocks", "1080/udp:udpsocks", "1080:udpsocks/udp",
		"stdio:udpsocks", "stdio:1080:udpsocks", "R:stdio:udpsocks",
	} {
		t.Run(input, func(t *testing.T) {
			if r, err := DecodeRemote(input); err == nil {
				t.Fatalf("invalid remote accepted: %#v", r)
			}
		})
	}
}
