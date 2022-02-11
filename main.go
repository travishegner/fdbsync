package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/vishvananda/netlink"
)

const (
	linkGlob            = "vx_*"
	containerCIDR       = "10.1.0.0/16"
	defaultSiblingTable = 193
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	done := make(chan struct{})
	addrUpdates := make(chan netlink.AddrUpdate, 1024)
	routeUpdates := make(chan netlink.RouteUpdate, 1024)

	err := netlink.AddrSubscribe(addrUpdates, done)
	if err != nil {
		log.Fatalf("failed to suscribe to addr changes: %v\n", err)
	}

	err = netlink.RouteSubscribe(routeUpdates, done)
	if err != nil {
		log.Fatalf("failed to subscribe to route changes: %v\n", err)
	}

	_, cnet, err := net.ParseCIDR(containerCIDR)
	if err != nil {
		log.Fatalf("failed to parse %v as container cidr: %v", containerCIDR, err)
	}

	addrs, err := netlink.AddrList(nil, netlink.FAMILY_ALL)
	if err != nil {
		log.Fatalf("failed to get address list")
	}

	for _, addr := range addrs {
		if !cnet.Contains(addr.IP) {
			continue
		}
		_, prefix, _ := net.ParseCIDR(addr.IPNet.String())
		err = syncFDB(prefix)
		if err != nil {
			log.Fatalf("failed to do initial fdb sync: %v\n", err)
		}
	}

Control:
	for {
		select {
		case au := <-addrUpdates:
			if !au.NewAddr {
				//only care about new addresses
				continue
			}

			if !cnet.Contains(au.LinkAddress.IP) {
				//only care about container networks
				continue
			}

			_, prefix, _ := net.ParseCIDR(au.LinkAddress.String())
			log.Printf("link added for %v\n", prefix)

			err := syncFDB(prefix)
			if err != nil {
				log.Printf("failed to sync fdb after address add: %v", err)
			}
		case ru := <-routeUpdates:
			if ru.Table != defaultSiblingTable {
				//not a route we care about
				continue
			}

			dc, err := isDirectlyConnected(ru.Dst)
			if err != nil {
				log.Printf("error checking if local route: %v\n", err)
			}
			if !dc {
				//not a route we care about
				continue
			}

			switch ru.Type {
			case syscall.RTM_NEWROUTE:
				log.Printf("route added for %v\n", ru.Dst)
				err := syncFDB(ru.Dst)
				if err != nil {
					log.Printf("failed to sync fdb after route add: %v", err)
				}
			case syscall.RTM_DELROUTE:
				log.Printf("route deleted for %v\n", ru.Dst)
				err := syncFDB(ru.Dst)
				if err != nil {
					log.Printf("failed to sync fdb after route del: %v", err)
				}
			default:
				continue
			}
		case s := <-sig:
			switch s {
			case syscall.SIGINT:
				fallthrough
			case syscall.SIGKILL:
				fmt.Printf("received %v signal, quitting\n", s)
				break Control
			}
		}
	}

	close(done)
	close(sig)

	fmt.Println("tetelestai")
}
