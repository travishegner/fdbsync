package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/travishegner/fdbsync/bird"
	"github.com/vishvananda/netlink"
)

const (
	birdSocket    = "/var/run/bird/bird.ctl"
	birdInterval  = 5 * time.Second
	containerCIDR = "10.1.0.0/16"
	linkGlob      = "mv_*"
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	done := make(chan struct{})
	addrUpdates := make(chan netlink.AddrUpdate, 1024)
	siblingUpdates := make(chan *SiblingUpdate, 1024)

	err := netlink.AddrSubscribe(addrUpdates, done)
	if err != nil {
		log.Fatalf("failed to subscribe to route changes: %v", err)
	}

	watchers := make(map[string]*SiblingWatcher)

	_, cnet, err := net.ParseCIDR(containerCIDR)
	if err != nil {
		log.Fatalf("failed to parse container CIDR: %v\n", containerCIDR)
	}

	addrs, err := netlink.AddrList(nil, netlink.FAMILY_ALL)
	if err != nil {
		log.Fatalf("failed to get system link list")
	}

	for _, addr := range addrs {
		if !cnet.Contains(addr.IP) {
			continue
		}

		_, prefix, _ := net.ParseCIDR(addr.IPNet.String())

		c, err := bird.NewSocketClient(birdSocket)
		if err != nil {
			log.Printf("failed to create new socket client for: %v\n", prefix)
			log.Printf("error: %v\n", err)
			continue
		}
		w := NewSiblingWatcher(
			birdInterval,
			c,
			prefix,
			siblingUpdates,
		)

		w.Watch()
		watchers[prefix.String()] = w
	}

Control:
	for {
		select {
		case au := <-addrUpdates:
			if !cnet.Contains(au.LinkAddress.IP) {
				continue
			}

			_, prefix, _ := net.ParseCIDR(au.LinkAddress.String())

			if !au.NewAddr {
				fmt.Printf("addr %v deleted\n", au.LinkAddress)

				//an overlay network was detached, stop and delete it's watcher
				w, ok := watchers[prefix.String()]
				if !ok {
					fmt.Printf("no watcher found for %v\n", prefix)
					continue
				}
				fmt.Printf("stopping watcher for %v\n", prefix)
				w.Stop()
				delete(watchers, prefix.String())

				continue
			}

			_, ok := watchers[prefix.String()]
			if ok {
				fmt.Printf("duplicate watcher found for %v refusing to start another one\n", prefix)
				continue
			}

			//a new overlay network attached, create and start a new watcher
			c, err := bird.NewSocketClient(birdSocket)
			if err != nil {
				log.Printf("failed to create new socket client for %v\n", prefix)
				log.Printf("error: %v\n", err)
				continue
			}
			w := NewSiblingWatcher(
				birdInterval,
				c,
				prefix,
				siblingUpdates,
			)
			watchers[prefix.String()] = w

			log.Printf("starting watcher for %v\n", prefix)
			w.Watch()
		case su := <-siblingUpdates:
			if su.Added {
				//detected a new sibling connected to an overlay
				log.Printf("adding sibling %v to fdb for %v", su.Sibling, su.Prefix)
				err := addSibling(su.Prefix, su.Sibling)
				if err != nil {
					log.Printf("failed to add sibling %v to forwarding database for %v\n", su.Sibling, su.Prefix)
					log.Printf("error: %v\n", err)
				}
				continue
			}
			//detected a sibling disconnecting from an overlay
			log.Printf("deleting sibling %v from fdb for %v", su.Sibling, su.Prefix)
			err := delSibling(su.Prefix, su.Sibling)
			if err != nil {
				log.Printf("failed to delete sibling %v from forwarding database for %v\n", su.Sibling, su.Prefix)
				log.Printf("error: %v\n", err)
			}
		case s := <-sig:
			switch s {
			case syscall.SIGUSR1:
				fmt.Println()
				fmt.Println("Dumping State Tables")
				fmt.Println("Watchers:")
				for k := range watchers {
					fmt.Printf("\t%v\n", k)
				}
				fmt.Println()
			case syscall.SIGINT:
				fallthrough
			case syscall.SIGKILL:
				fmt.Printf("received %v signal, quitting\n", s)
				break Control
			default:
				continue
			}
		}
	}

	for _, watcher := range watchers {
		watcher.Stop()
	}

	close(done)
	close(sig)

	fmt.Println("tetelestai")
}
