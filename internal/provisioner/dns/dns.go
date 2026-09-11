// Package dns points a hostname at a rotated address.
//
// Pointing client configs at a name rather than a raw IP is what makes rotation
// invisible: REALITY treats the connect address and the camouflage SNI as
// independent fields, so swapping the A record behind n1.example.com changes
// nothing a client has to re-import. Failover becomes DNS TTL plus a reconnect.
package dns

import "context"

// Updater points a fully-qualified name at an address.
type Updater interface {
	// Name identifies the provider in logs and output.
	Name() string

	// Upsert makes fqdn resolve to ip, creating the record if it is missing.
	Upsert(ctx context.Context, fqdn, ip string) error
}
