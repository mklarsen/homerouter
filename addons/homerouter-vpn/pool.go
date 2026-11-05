package main

import (
	"errors"
	"fmt"
	"net/netip"
)

// Pool hands out peer addresses from an IPv4 subnet. The first host belongs
// to the server, peers get the following ones.
type Pool struct {
	prefix netip.Prefix
	server netip.Addr
}

func NewPool(cidr string) (*Pool, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet %q: %w", cidr, err)
	}
	if !p.Addr().Is4() {
		return nil, errors.New("subnet must be IPv4")
	}
	if p.Bits() > 29 {
		return nil, errors.New("subnet must be /29 or larger")
	}
	p = p.Masked()
	return &Pool{prefix: p, server: p.Addr().Next()}, nil
}

func (p *Pool) Prefix() netip.Prefix { return p.prefix }

// ServerAddr is the server's address with the subnet length, e.g. 10.10.30.1/24.
func (p *Pool) ServerAddr() netip.Prefix { return netip.PrefixFrom(p.server, p.prefix.Bits()) }

// Allocate returns the lowest free peer address, or an error when exhausted.
func (p *Pool) Allocate(used map[netip.Addr]bool) (netip.Addr, error) {
	for a := p.server.Next(); p.prefix.Contains(a); a = a.Next() {
		// The last address of the subnet is the broadcast address.
		if !p.prefix.Contains(a.Next()) {
			break
		}
		if !used[a] {
			return a, nil
		}
	}
	return netip.Addr{}, errors.New("address pool exhausted")
}
