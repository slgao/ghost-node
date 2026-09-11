// Package provisioner rotates the public IP addresses of VPN nodes.
//
// On Oracle Cloud's Always Free tier you cannot provision a replacement VM when
// an address is blocked — Always Free instances are pinned to your tenancy's
// home region, capacity is routinely unavailable, and the quota is fixed. What
// you can do, for free and in under a minute, is release an instance's
// ephemeral public IP and draw a new one from Oracle's regional pool. That is
// the remedy this package automates.
package provisioner

import (
	"context"
	"errors"
)

// ErrReservedIP is returned when an instance carries a reserved (rather than
// ephemeral) public IP. Rotating those means unassign/reassign against a fixed
// address, which defeats the purpose — the whole point is a different address.
var ErrReservedIP = errors.New("instance has a RESERVED public IP; rotation needs an EPHEMERAL one")

// ErrNoPublicIP is returned when an instance has no public IP to rotate.
var ErrNoPublicIP = errors.New("instance has no public IP attached")

// Provider is a cloud that can change an instance's public address.
// Oracle is the only implementation today; the interface exists so a paid
// provider can be added later for ASN diversity without touching callers.
type Provider interface {
	// Name identifies the provider in logs and output.
	Name() string

	// CurrentIP returns the instance's current public address.
	CurrentIP(ctx context.Context, instanceID string) (string, error)

	// RotateIP releases the current public address and allocates a new one,
	// returning the old and new addresses. The instance keeps running and its
	// VPN process needs no restart: it listens on 0.0.0.0 and the address is
	// translated upstream.
	RotateIP(ctx context.Context, instanceID string) (oldIP, newIP string, err error)
}
