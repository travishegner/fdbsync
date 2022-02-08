package bird

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type SiblingWatcher struct {
	client   Client
	interval time.Duration
	state    map[string]map[string]struct{}
	dones    map[string]chan struct{}
}

type SiblingUpdate struct {
	Prefix  *net.IPNet
	Address net.IP
	Added   bool
}

func NewSiblingWatcher(client Client, interval time.Duration) *SiblingWatcher {
	return &SiblingWatcher{
		client:   client,
		interval: interval,
		state:    make(map[string]map[string]struct{}),
	}
}

func (sw *SiblingWatcher) Watch(prefix *net.IPNet, ch chan *SiblingUpdate) {
	sw.dones[prefix.String()] = make(chan struct{})
	go sw.watch(prefix, ch)
}

func (sw *SiblingWatcher) StopWatch(prefix *net.IPNet) {
	if ch, ok := sw.dones[prefix.String()]; ok {
		close(ch)
	}
}

func (sw *SiblingWatcher) watch(prefix *net.IPNet, ch chan *SiblingUpdate) {
	timer := time.NewTimer(sw.interval)

	for {
		select {
		case <-timer.C:
			updates, err := sw.reconcile(prefix)
			if err != nil {
				log.Printf("error while watching bird: %v\n", err)
				log.Printf("state may be out of sync\n")
			}
			for _, update := range updates {
				ch <- update
			}
		case <-sw.dones[prefix.String()]:
			log.Printf("done channel closed for prefix %v\n", prefix)
			close(ch)
			return
		}
	}
}

func (sw *SiblingWatcher) reconcile(prefix *net.IPNet) ([]*SiblingUpdate, error) {
	updates := make([]*SiblingUpdate, 0)

	news, err := sw.newSiblings(prefix)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrive new sibling set")
	}

	olds, ok := sw.state[prefix.String()]
	if !ok {
		olds = make(map[string]struct{})
	}

	for sibling := range news {
		_, ok := olds[sibling]
		if ok {
			continue
		}
		updates = append(updates, &SiblingUpdate{
			Prefix:  prefix,
			Address: net.ParseIP(sibling),
			Added:   true,
		})
	}

	for sibling := range olds {
		_, ok := news[sibling]
		if ok {
			continue
		}
		updates = append(updates, &SiblingUpdate{
			Prefix:  prefix,
			Address: net.ParseIP(sibling),
			Added:   false,
		})
	}

	sw.state[prefix.String()] = news

	return updates, nil
}

func (sw *SiblingWatcher) newSiblings(prefix *net.IPNet) (map[string]struct{}, error) {
	siblings := make(map[string]struct{})
	lines, err := sw.client.Query(fmt.Sprintf("show route %v where source = RTS_BGP", prefix))
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
