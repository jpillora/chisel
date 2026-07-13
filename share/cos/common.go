package cos

import (
	"context"
	"os"
	"os/signal"
	"time"
	"syscall"
	"log"
)

//InterruptContext returns a context which is
//cancelled on OS Interrupt
func InterruptContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM) //windows compatible?
		s := <-sig
		log.Printf("Graceful shutdown signal received: %s", s)
		signal.Stop(sig)
		log.Println("Graceful shutdown complete.")
		cancel()
	}()
	return ctx
}

//SleepSignal sleeps for the given duration,
//or until a SIGHUP is received
func SleepSignal(d time.Duration) {
	<-AfterSignal(d)
}
