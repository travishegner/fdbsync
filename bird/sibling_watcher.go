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

func (sw *SiblingWatcher) Watch(prefix *net.IPNet, ch chan SiblingUpdate, done chan struct{}) {
	go sw.watch(prefix, ch, done)
}

func (sw *SiblingWatcher) watch(prefix *net.IPNet, ch chan SiblingUpdate, done chan struct{}) {
	timer := time.NewTimer(sw.interval)

	for {
		select {
		case <-timer.C:
			updates, err := sw.reconcile(prefix)
			if err != nil {
				log.Fatalf("error watching bird: %v", err)
			}
			for _, update := range updates {
				ch <- update
			}
		case <-done:
			log.Println("done channel closed")
			return
		}
	}
}

func (sw *SiblingWatcher) reconcile(prefix *net.IPNet) ([]SiblingUpdate, error) {
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
		fmt.Println(address)
	}

	return nil, nil
}
