package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/opts"
)

// helpText renders a command's help at a fixed total line width, bypassing
// terminal detection so the wrapping is deterministic under `go test`.
func helpText(t *testing.T, cmd opts.Opts, width int) string {
	t.Helper()
	_, err := cmd.SetLineWidth(width).ParseArgsError([]string{"chisel", "--help"})
	if err == nil {
		t.Fatal("help did not stop parsing")
	}
	return err.Error()
}

// optionLines returns the flag list, which is the section opts wraps. The
// surrounding blocks (remotes, signals) are rendered verbatim by design.
func optionLines(help string) []string {
	lines := strings.Split(help, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "Options:" {
			continue
		}
		for j, line := range lines[i+1:] {
			if strings.TrimSpace(line) == "" {
				return lines[i+1 : i+1+j]
			}
		}
		return lines[i+1:]
	}
	return nil
}

func helpCommands(t *testing.T) map[string]func() opts.Opts {
	t.Helper()
	return map[string]func() opts.Opts{
		"server": func() opts.Opts { return newServerCLI(newServerOptions()) },
		"client": func() opts.Opts { return newClientCLI(newClientOptions()) },
	}
}

// The help tags hold unwrapped prose, so opts is free to wrap them to whatever
// width it detects. Words longer than the width are left to overflow.
func TestFlagHelpWrapsToLineWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	const padWidth = 2
	for name, newCmd := range helpCommands(t) {
		for _, width := range []int{60, 80, 96} {
			t.Run(fmt.Sprintf("%s/%d", name, width), func(t *testing.T) {
				lines := optionLines(helpText(t, newCmd(), width))
				if len(lines) == 0 {
					t.Fatal("no option lines rendered")
				}
				for _, line := range lines {
					if len(line) <= width-padWidth {
						continue
					}
					if fields := strings.Fields(line); len(fields) == 1 {
						continue //a single unbreakable word
					}
					t.Errorf("line exceeds width %d (%d chars): %q", width, len(line), line)
				}
			})
		}
	}
}

func TestFlagHelpReflowsWithWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for name, newCmd := range helpCommands(t) {
		t.Run(name, func(t *testing.T) {
			narrow := optionLines(helpText(t, newCmd(), 60))
			wide := optionLines(helpText(t, newCmd(), 96))
			if len(narrow) <= len(wide) {
				t.Fatalf("help did not reflow: %d lines at width 60, %d at width 96", len(narrow), len(wide))
			}
		})
	}
}

// Every flag carrying a help tag must survive into the rendered help.
func TestHelpListsEveryFlag(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	want := map[string][]string{
		"server": {
			"--host", "--port", "--keygen", "--key", "--keyfile", "--authfile",
			"--auth", "--backend", "--socks5", "--reverse", "--keepalive",
			"--tls-key", "--tls-cert", "--tls-domain", "--tls-ca", "--proxy",
			"--pid", "--verbose",
		},
		"client": {
			"--fingerprint", "--auth", "--keepalive", "--max-retry-count",
			"--min-retry-interval", "--max-retry-interval", "--proxy",
			"--tls-skip-verify", "--tls-ca", "--tls-cert", "--tls-key",
			"--verbose", "--header", "--hostname", "--sni", "--pid",
		},
	}
	for name, newCmd := range helpCommands(t) {
		t.Run(name, func(t *testing.T) {
			help := strings.Join(optionLines(helpText(t, newCmd(), 96)), "\n")
			for _, flag := range want[name] {
				//a flag with a short name renders as "--verbose, -v"
				if !strings.Contains(help, flag+" ") && !strings.Contains(help, flag+",") {
					t.Errorf("%s missing from help", flag)
				}
			}
		})
	}
}

func TestAllFlagsHaveHelpTags(t *testing.T) {
	for _, config := range []any{serverOptions{}, clientOptions{}} {
		checkFlagHelpTags(t, reflect.TypeOf(config))
	}
}

func checkFlagHelpTags(t *testing.T, typeOf reflect.Type) {
	t.Helper()
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if field.PkgPath != "" {
			continue
		}
		optsTag := field.Tag.Get("opts")
		if optsTagValue(optsTag, "mode") == "arg" {
			continue
		}
		if field.Tag.Get("help") != "" {
			continue
		}
		if field.Type.Kind() == reflect.Struct && optsTag != "-" {
			checkFlagHelpTags(t, field.Type)
			continue
		}
		if optsTag != "-" {
			t.Errorf("%s.%s has no help tag", typeOf.Name(), field.Name)
		}
	}
}

func TestServerOptions(t *testing.T) {
	server := newServerOptions()
	cli := newCLI(server, newClientOptions())
	_, err := cli.ParseArgsError([]string{
		"chisel", "server",
		"--host", "127.0.0.1",
		"-p", "9090",
		"--key", "seed",
		"--keyfile", "key.pem",
		"--authfile", "users.json",
		"--auth", "user:pass",
		"--keepalive", "30s",
		"--backend", "http://first",
		"--proxy", "http://last",
		"--socks5",
		"--reverse",
		"--tls-key", "tls-key.pem",
		"--tls-cert", "tls-cert.pem",
		"--tls-domain", "one.example",
		"--tls-domain", "two.example",
		"--tls-ca", "ca.pem",
		"--pid",
		"-v",
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.Host != "127.0.0.1" || server.Port != "9090" {
		t.Fatalf("listen options not parsed: host=%q port=%q", server.Host, server.Port)
	}
	if server.Config.Proxy != "http://last" {
		t.Fatalf("proxy alias order not preserved: %q", server.Config.Proxy)
	}
	if server.KeepAlive != 30*time.Second || !server.Socks5 || !server.Reverse || !server.PID || !server.Verbose {
		t.Fatalf("server options not parsed: %+v", server)
	}
	if want := []string{"one.example", "two.example"}; !reflect.DeepEqual(server.Config.TLS.Domains, want) {
		t.Fatalf("TLS domains: got %q, want %q", server.Config.TLS.Domains, want)
	}
}

func TestClientOptions(t *testing.T) {
	client := newClientOptions()
	cli := newCLI(newServerOptions(), client)
	_, err := cli.ParseArgsError([]string{
		"chisel", "client",
		"--fingerprint", "fingerprint",
		"--auth", "user:pass",
		"--keepalive", "15s",
		"--max-retry-count", "7",
		"--min-retry-interval", "2s",
		"--max-retry-interval", "1m",
		"--proxy", "socks5://localhost:1080",
		"--header", "Foo: one",
		"--header", "Bar: two",
		"--hostname", "host.example",
		"--sni", "sni.example",
		"--tls-ca", "ca.pem",
		"--tls-skip-verify",
		"--tls-key", "key.pem",
		"--tls-cert", "cert.pem",
		"--pid",
		"-v",
		"wss://server.example",
		"3000",
		"R:2222:localhost:22",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.Server != "wss://server.example" {
		t.Fatalf("server: %q", client.Server)
	}
	if want := []string{"3000", "R:2222:localhost:22"}; !reflect.DeepEqual(client.Remotes, want) {
		t.Fatalf("remotes: got %q, want %q", client.Remotes, want)
	}
	if want := []string{"Foo: one", "Bar: two"}; !reflect.DeepEqual(client.Header, want) {
		t.Fatalf("headers: got %q, want %q", client.Header, want)
	}
	if client.KeepAlive != 15*time.Second || client.MaxRetryCount != 7 || !client.Config.TLS.SkipVerify || !client.PID || !client.Verbose {
		t.Fatalf("client options not parsed: %+v", client)
	}
}
