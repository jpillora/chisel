//go:build !windows

package cos

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// TestInterruptContextSIGTERM verifies that SIGTERM (docker stop,
// kubernetes) cancels the context rather than killing the process.
func TestInterruptContextSIGTERM(t *testing.T) {
	ctx := InterruptContext()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		//graceful path taken
	case <-time.After(2 * time.Second):
		t.Fatal("SIGTERM did not cancel the context")
	}
}
