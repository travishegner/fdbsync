package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const (
	birdSocket = "/var/run/bird/bird.ctl"
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

Control:
	for {
		select {
		case s := <-sig:
			fmt.Printf("received %v signal, quitting\n", s)
			break Control
		}
	}

	close(sig)
}
