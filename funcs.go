package main

import (
	"fmt"
	"net"
	"syscall"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

func addSibling(prefix *net.IPNet, sibling net.IP) error {
	vxid, err := vxlanIDFromPrefix(prefix)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get vx link for vxlan: %v", vxid))
	}
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

func delSibling(prefix *net.IPNet, sibling net.IP) error {
	vxid, err := vxlanIDFromPrefix(prefix)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get vxid from prefix %v", prefix))
	}
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

	return errors.New(fmt.Sprintf("no neighbor entry %v found for vxlan: %v", sibling, vxid))
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
