package main

import (
	"fmt"
	"log"
	"net"
	"syscall"

	"github.com/TrilliumIT/iputil"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

func addSibling(vxid int, sibling net.IP) error {
	mac, _ := net.ParseMAC("00:00:00:00:00:00")
	neigh := &netlink.Neigh{
		LinkIndex:    vxid,
		State:        netlink.NUD_PERMANENT,
		Family:       syscall.AF_BRIDGE,
		Flags:        netlink.NTF_SELF,
		IP:           sibling,
		HardwareAddr: mac,
	}

	return netlink.NeighAppend(neigh)
}

func delSibling(vxid int, sibling net.IP) error {
	neighs, err := netlink.NeighList(vxid, syscall.AF_BRIDGE)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get the bridge neighbors for vxlan: %v", vxid))
	}
	for _, neigh := range neighs {
		if !neigh.IP.Equal(sibling) {
			continue
		}

		if neigh.HardwareAddr.String() != "00:00:00:00:00:00" {
			continue
		}

		err = netlink.NeighDel(&neigh)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to delete neighbor entry %v for vxlan: %v", sibling, vxid))
		}
		return nil
	}

	return nil
}

func vxlanIDFromPrefix(prefix *net.IPNet) (int, error) {
	routes, err := netlink.RouteGet(prefix.IP)
	if err != nil {
		return -1, errors.New(fmt.Sprintf("failed to get route for %v\n", prefix.IP))
	}
	for _, route := range routes {
		if route.Gw != nil {
			continue
		}
		mvl, err := netlink.LinkByIndex(route.LinkIndex)
		if err != nil {
			return -1, errors.New(fmt.Sprintf("failed to get mvl by index %v", route.LinkIndex))
		}

		return mvl.Attrs().ParentIndex, nil
	}

	return -1, errors.New(fmt.Sprintf("failed to get device route for %v\n", prefix))
}

func isDirectlyConnected(prefix *net.IPNet) (bool, error) {
	addrs, err := netlink.AddrList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return false, err
	}

	for _, addr := range addrs {
		if prefix.Contains(addr.IP) {
			return true, nil
		}
	}

	return false, nil
}

func getRouteSiblings(prefix *net.IPNet) (map[string]net.IP, error) {
	routes, err := netlink.RouteListFiltered(
		netlink.FAMILY_ALL,
		&netlink.Route{
			Table: defaultSiblingTable,
		},
		netlink.RT_FILTER_TABLE,
	)
	if err != nil {
		return nil, err
	}

	routeSiblings := make(map[string]net.IP)
	for _, route := range routes {
		if !iputil.SubnetEqualSubnet(route.Dst, prefix) {
			continue
		}

		if route.Gw != nil {
			routeSiblings[route.Gw.String()] = route.Gw
			continue
		}

		for _, nh := range route.MultiPath {
			routeSiblings[nh.Gw.String()] = nh.Gw
		}
	}

	return routeSiblings, nil
}

func getFDBSiblings(prefix *net.IPNet) (map[string]net.IP, error) {
	vxid, err := vxlanIDFromPrefix(prefix)
	if err != nil {
		return nil, err
	}

	fdb := make(map[string]net.IP)
	neighs, err := netlink.NeighList(vxid, syscall.AF_BRIDGE)
	if err != nil {
		return nil, err
	}

	for _, neigh := range neighs {
		if neigh.HardwareAddr.String() != "00:00:00:00:00:00" {
			continue
		}
		fdb[neigh.IP.String()] = neigh.IP
	}

	return fdb, nil
}

func syncFDB(prefix *net.IPNet) error {
	newSibs, err := getRouteSiblings(prefix)
	if err != nil {
		return err
	}

	oldSibs, err := getFDBSiblings(prefix)
	if err != nil {
		return err
	}

	vxid, err := vxlanIDFromPrefix(prefix)
	if err != nil {
		return err
	}

	for key, sib := range newSibs {
		_, ok := oldSibs[key]
		if ok {
			continue
		}
		log.Printf("adding sibling %v to fdb for %v\n", sib, prefix)
		err := addSibling(vxid, sib)
		if err != nil {
			log.Printf("failed to add sibling %v to fdb: %v\n", sib, err)
		}
	}

	for key, sib := range oldSibs {
		_, ok := newSibs[key]
		if ok {
			continue
		}
		log.Printf("deleting sibling %v from fdb for %v\n", sib, prefix)
		err := delSibling(vxid, sib)
		if err != nil {
			log.Printf("failed to delete sibling %v from fdb: %v\n", sib, err)
		}
	}

	return nil
}
