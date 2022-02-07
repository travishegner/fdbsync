package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vishvananda/netlink"
)

func main() {
	routeUpdates := make(chan netlink.RouteUpdate, 1024)
	done := make(chan struct{})

	err := netlink.RouteSubscribe(routeUpdates, done)
	if err != nil {
		log.Fatalf("failed to subscribe to route updates: %v", err)
	}

	sig := make(chan os.Signal, 1)

	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

Control:
	for {
		select {
		case ru := <-routeUpdates:
			fmt.Println("route update: ", ru)
		case s := <-sig:
			fmt.Printf("received %v signal, quitting\n", s)
			break Control
		}
	}

	close(done)
}
