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
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var sliverArgs sync.Mutex

// entrypoint implements the Sliver native extension ABI:
//
//	int32_t entrypoint(char *buffer, uint32_t size, callback(char *, int32_t))
//
// chisel's tunnel runs until it is stopped, so it is started in the background
// and the call returns immediately. The callback is only valid during this
// call, hence the brief wait to report an immediate argument or connection
// error.
//
//export entrypoint
func entrypoint(buffer *C.char, size C.uint32_t, callback C.sliver_callback_t) C.int32_t {
	args := ""
	if buffer != nil && size > 0 {
		args = C.GoStringN(buffer, C.int(size))
	}

	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("chisel panic: %v", r)
			}
		}()
		errCh <- runFromSliver(strings.Fields(args))
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return respond(callback, "chisel error: "+err.Error(), 1)
		}
		return respond(callback, "chisel exited", 0)
	case <-time.After(2 * time.Second):
		return respond(callback, "chisel started: "+args, 0)
	}
}

// runFromSliver runs the normal CLI with the Sliver-supplied arguments. Unlike
// main() it returns errors instead of calling os.Exit, which would take down
// the implant process the extension is loaded into.
func runFromSliver(args []string) error {
	sliverArgs.Lock()
	os.Args = append([]string{"chisel"}, args...)
	serverConfig := newServerOptions()
	clientConfig := newClientOptions()
	cli := newCLI(serverConfig, clientConfig)
	parsed, err := cli.ParseArgsError(os.Args)
	sliverArgs.Unlock()
	if err != nil {
		return err
	}
	if !parsed.IsRunnable() {
		return fmt.Errorf("no command specified")
	}
	return parsed.Run()
}

func respond(callback C.sliver_callback_t, msg string, rc C.int32_t) C.int32_t {
	if callback != nil {
		cs := C.CString(msg)
		defer C.free(unsafe.Pointer(cs))
		C.sliver_call_callback(callback, cs, C.int32_t(len(msg)))
	}
	return rc
}
