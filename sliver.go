//go:build cgo

package main

import "C"

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// Sliver loads aliases on Windows by wrapping the DLL in Donut, which calls the
// manifest's entrypoint export as `void entrypoint(char *args)` — a single
// command-line string, no callback. The arguments are the same ones you would
// pass to the chisel binary.
//
//export entrypoint
func entrypoint(args *C.char) {
	argstring := ""
	if args != nil {
		argstring = C.GoString(args)
	}
	// Sliver kills the host process as soon as this returns, so run the tunnel
	// on this thread and only come back when chisel exits.
	runFromSliver(strings.Fields(argstring))
}

// runFromSliver runs chisel with the Sliver-supplied arguments. Unlike main()
// it never calls os.Exit, so a bad command line or a dead tunnel cannot take
// down the process the DLL is loaded into.
func runFromSliver(args []string) {
	os.Args = append([]string{"chisel"}, args...)

	serverConfig := newServerOptions()
	clientConfig := newClientOptions()
	cli := newCLI(serverConfig, clientConfig)

	parsed, err := cli.ParseArgsError(os.Args)
	if err != nil {
		// Help and version are modelled as parse errors by opts; both are
		// harmless here, as is a malformed command line.
		fmt.Print(err)
		return
	}
	if !parsed.IsRunnable() {
		fmt.Print(parsed.Help())
		return
	}
	if err := parsed.Run(); err != nil {
		log.Print(err)
	}
}
