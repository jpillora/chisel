package chshare

import (
	"reflect"
	"testing"
)

func TestRemoteDecode(t *testing.T) {
	//test table
	for i, test := range []struct {
		Input  string
		Output Remote
	}{
		{
			"3000",
			Remote{
				LocalPort:  "3000",
				RemoteHost: "localhost",
				RemotePort: "3000",
			},
		},
		{
			"google.com:80",
			Remote{
				LocalPort:  "80",
				RemoteHost: "google.com",
				RemotePort: "80",
			},
		},
		{
			"R:google.com:80",
			Remote{
				LocalPort:  "80",
				RemoteHost: "google.com",
				RemotePort: "80",
				Reverse:    true,
			},
		},
		{
			"socks",
			Remote{
				LocalHost: "127.0.0.1",
				LocalPort: "1080",
				Socks:     true,
			},
		},
		{
			"127.0.0.1:1081:socks",
			Remote{
				LocalHost: "127.0.0.1",
				LocalPort: "1081",
				Socks:     true,
			},
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
		},
		{
			"localhost:5353/tcp:1.1.1.1:53/udp",
			Remote{
				LocalHost:   "localhost",
				LocalPort:   "5353",
				LocalProto:  "tcp",
				RemoteHost:  "1.1.1.1",
				RemotePort:  "53",
				RemoteProto: "udp",
			},
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
	}
}
