package cos

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//InterruptContext returns a context which is cancelled on OS
//interrupt or SIGTERM (docker stop, kubernetes pod termination).
//A second signal forces an immediate exit.
func InterruptContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	//register before returning so there is no window where a
	//SIGTERM takes the default kill path
	//(SIGTERM never fires on windows, os.Interrupt covers ctrl-c)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
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
