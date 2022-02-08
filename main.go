package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gobwas/glob"
	"github.com/travishegner/fdbsync/bird"
	"github.com/vishvananda/netlink"
)

const (
	birdSocket       = "/var/run/bird/bird.ctl"
	birdInterval     = 5 * time.Second
	interfacePattern = "mv_*"
	containerCIDR    = "10.1.0.0/16"
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	linkUpdates := make(chan netlink.LinkUpdate, 1024)
	siblingUpdates := make(chan *bird.SiblingUpdate, 1024)

	err := netlink.LinkSubscribe(linkUpdates, done)
	if err != nil {
		log.Fatalf("failed to subscribe to link changes: %v", err)
	}

	links, err := netlink.LinkList()
	if err != nil {
		log.Fatalf("failed to get current links")
	}

	globber := glob.MustCompile(interfacePattern)
	birdClient, err := bird.NewSocketClient(birdSocket)
	if err != nil {
		log.Fatalf("failed to get bird client: %v\n", err)
	}
	watcher := bird.NewSiblingWatcher(birdClient, birdInterval)

	for _, link := range links {
		if !globber.Match(link.Attrs().Name) {
			continue
		}
		addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
		if err != nil {
			log.Printf("failed to get addresses for link %v: %v", link.Attrs().Name, err)
			continue
		}
		for _, addr := range addrs {
			_, net, err := net.ParseCIDR(addr.IPNet.String())
			if err != nil {
				log.Printf("failed to get network id for link %v: %v", link.Attrs().Name, err)
				continue
			}
			watcher.Watch(net, siblingUpdates)
		}
	}

Control:
	for {
		select {
		case lu := <-linkUpdates:
			fmt.Println(lu)
		case su := <-siblingUpdates:
			fmt.Println(su)
		case s := <-sig:
			fmt.Printf("received %v signal, quitting\n", s)
			break Control
		}
	}

	close(sig)
}
