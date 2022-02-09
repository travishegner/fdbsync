package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/travishegner/fdbsync/bird"
	"github.com/vishvananda/netlink"
)

type SiblingWatcher struct {
	interval      time.Duration
	client        bird.Client
	done          chan struct{}
	prefix        *net.IPNet
	updateChannel chan *SiblingUpdate
}

type SiblingUpdate struct {
	Prefix  *net.IPNet
	Sibling net.IP
	Added   bool
}

func NewSiblingWatcher(interval time.Duration, client bird.Client, prefix *net.IPNet, ch chan *SiblingUpdate) *SiblingWatcher {
	return &SiblingWatcher{
		client:        client,
		interval:      interval,
		prefix:        prefix,
		updateChannel: ch,
	}
}

func (sw *SiblingWatcher) Watch() {
	sw.done = make(chan struct{})
	go sw.watch()
}

func (sw *SiblingWatcher) Stop() {
	close(sw.done)
	sw.client.Close()
}

func (sw *SiblingWatcher) watch() {
	err := sw.sendUpdates()
	if err != nil {
		log.Printf("failed to do initial sibling sync for %v\n", sw.prefix)
		log.Printf("%v\n\n", err)
		return
	}
	ticker := time.NewTicker(sw.interval)

	for {
		select {
		case <-ticker.C:
			err := sw.sendUpdates()
			if err != nil {
				log.Printf("failed to do scheduled sibling sync for %v\n", sw.prefix)
				log.Printf("%v\n\n", err)
			}
		case <-sw.done:
			log.Printf("done channel closed for prefix %v\n", sw.prefix)
			return
		}
	}
}

func (sw *SiblingWatcher) sendUpdates() error {
	updates, err := sw.reconcile()
	if err != nil {
		return errors.Wrap(err, "error while reconciling siblings")
	}
	for _, update := range updates {
		sw.updateChannel <- update
	}

	return nil
}

func (sw *SiblingWatcher) reconcile() ([]*SiblingUpdate, error) {
	updates := make([]*SiblingUpdate, 0)

	news, err := sw.newSiblings()
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrive new sibling set")
	}

	olds, err := sw.oldSiblings()
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve old sibling set")
	}

	for sibling := range news {
		_, ok := olds[sibling]
		if ok {
			continue
		}
		updates = append(updates, &SiblingUpdate{
			Prefix:  sw.prefix,
			Sibling: net.ParseIP(sibling),
			Added:   true,
		})
	}

	for sibling := range olds {
		_, ok := news[sibling]
		if ok {
			continue
		}
		updates = append(updates, &SiblingUpdate{
			Prefix:  sw.prefix,
			Sibling: net.ParseIP(sibling),
			Added:   false,
		})
	}

	return updates, nil
}

func (sw *SiblingWatcher) newSiblings() (map[string]struct{}, error) {
	siblings := make(map[string]struct{})
	lines, err := sw.client.Query(fmt.Sprintf("show route %v where source = RTS_BGP", sw.prefix))
	if err != nil {
		return nil, errors.Wrap(err, "error getting sibling routes from bird")
	}

	for _, line := range lines {
		split1 := strings.Split(line, "via")
		if len(split1) < 2 {
			continue
		}
		split2 := strings.Split(split1[1], "on")
		address := strings.TrimSpace(split2[0])
		siblings[address] = struct{}{}
	}

	return siblings, nil
}

func (sw *SiblingWatcher) oldSiblings() (map[string]struct{}, error) {
	olds := make(map[string]struct{})

	vxid, err := vxlanIDFromPrefix(sw.prefix)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get vxid from prefix %v", sw.prefix))
	}

	neighs, err := netlink.NeighList(vxid, syscall.AF_BRIDGE)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get the bridge neighbors for vxlan: %v", vxid))
	}
	for _, neigh := range neighs {
		olds[neigh.IP.String()] = struct{}{}
	}

	return olds, nil
}
