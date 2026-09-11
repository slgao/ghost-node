package provisioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vpnplatform/core/internal/provisioner/ociclient"
)

// Oracle rotates ephemeral public IPs on OCI compute instances.
type Oracle struct {
	client *ociclient.Client

	// ReleaseTimeout bounds the wait for a released IP to detach.
	ReleaseTimeout time.Duration
	// AssignTimeout bounds the wait for a new IP to reach ASSIGNED.
	AssignTimeout time.Duration
	// PollInterval is how often lifecycle state is re-checked.
	PollInterval time.Duration

	// Logf receives progress messages. Optional.
	Logf func(format string, args ...any)
}

func NewOracle(client *ociclient.Client) *Oracle {
	return &Oracle{
		client:         client,
		ReleaseTimeout: 90 * time.Second,
		AssignTimeout:  120 * time.Second,
		PollInterval:   3 * time.Second,
	}
}

func (o *Oracle) Name() string { return "oracle/" + o.client.Region() }

func (o *Oracle) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// binding is everything needed to swap an address: the primary VNIC's primary
// private IP, plus whatever public IP is currently sitting on it.
type binding struct {
	instance  *ociclient.Instance
	privateIP *ociclient.PrivateIP
	publicIP  *ociclient.PublicIP // nil when none is attached
}

func (o *Oracle) describe(ctx context.Context, instanceOCID string) (*binding, error) {
	inst, err := o.client.GetInstance(ctx, instanceOCID)
	if err != nil {
		return nil, fmt.Errorf("fetching instance: %w", err)
	}

	attachments, err := o.client.ListVnicAttachments(ctx, inst.CompartmentID, inst.ID)
	if err != nil {
		return nil, fmt.Errorf("listing VNIC attachments: %w", err)
	}

	var primary *ociclient.Vnic
	for _, att := range attachments {
		if att.LifecycleState != "ATTACHED" || att.VnicID == "" {
			continue
		}
		vnic, err := o.client.GetVnic(ctx, att.VnicID)
		if err != nil {
			return nil, fmt.Errorf("fetching VNIC %s: %w", att.VnicID, err)
		}
		if vnic.IsPrimary {
			primary = vnic
			break
		}
	}
	if primary == nil {
		return nil, errors.New("instance has no attached primary VNIC")
	}

	privateIPs, err := o.client.ListPrivateIPs(ctx, primary.ID)
	if err != nil {
		return nil, fmt.Errorf("listing private IPs: %w", err)
	}
	var privateIP *ociclient.PrivateIP
	for i := range privateIPs {
		if privateIPs[i].IsPrimary {
			privateIP = &privateIPs[i]
			break
		}
	}
	if privateIP == nil {
		return nil, errors.New("primary VNIC has no primary private IP")
	}

	b := &binding{instance: inst, privateIP: privateIP}

	publicIP, err := o.client.GetPublicIPByPrivateIPID(ctx, privateIP.ID)
	switch {
	case err == nil:
		b.publicIP = publicIP
	case ociclient.IsNotFound(err):
		// No public IP attached — RotateIP will allocate the first one.
	default:
		return nil, fmt.Errorf("fetching current public IP: %w", err)
	}

	return b, nil
}

// CurrentIP returns the instance's current public address.
func (o *Oracle) CurrentIP(ctx context.Context, instanceOCID string) (string, error) {
	b, err := o.describe(ctx, instanceOCID)
	if err != nil {
		return "", err
	}
	if b.publicIP == nil {
		return "", ErrNoPublicIP
	}
	return b.publicIP.IPAddress, nil
}

// Describe returns a human-readable summary of an instance for status output.
func (o *Oracle) Describe(ctx context.Context, instanceOCID string) (name, state, ip, lifetime string, err error) {
	b, err := o.describe(ctx, instanceOCID)
	if err != nil {
		return "", "", "", "", err
	}
	if b.publicIP != nil {
		ip, lifetime = b.publicIP.IPAddress, b.publicIP.Lifetime
	}
	return b.instance.DisplayName, b.instance.LifecycleState, ip, lifetime, nil
}

// RotateIP releases the instance's ephemeral public IP and allocates a new one.
func (o *Oracle) RotateIP(ctx context.Context, instanceOCID string) (string, string, error) {
	b, err := o.describe(ctx, instanceOCID)
	if err != nil {
		return "", "", err
	}

	oldIP := ""
	if b.publicIP != nil {
		if b.publicIP.Lifetime == ociclient.LifetimeReserved {
			return "", "", ErrReservedIP
		}
		oldIP = b.publicIP.IPAddress

		o.logf("releasing %s (%s)", oldIP, b.publicIP.ID)
		if err := o.client.DeletePublicIP(ctx, b.publicIP.ID); err != nil && !ociclient.IsNotFound(err) {
			return oldIP, "", fmt.Errorf("releasing public IP %s: %w", oldIP, err)
		}
		if err := o.waitReleased(ctx, b.privateIP.ID); err != nil {
			return oldIP, "", err
		}
	}

	fresh, err := o.allocate(ctx, b)
	if err != nil {
		return oldIP, "", err
	}

	newIP, err := o.waitAssigned(ctx, fresh.ID)
	if err != nil {
		return oldIP, "", err
	}

	o.logf("allocated %s", newIP)
	return oldIP, newIP, nil
}

// allocate requests a new ephemeral IP, tolerating the brief window where OCI
// still considers the released address attached.
func (o *Oracle) allocate(ctx context.Context, b *binding) (*ociclient.PublicIP, error) {
	name := "ghost-" + b.instance.DisplayName

	compartment := b.privateIP.CompartmentID
	if compartment == "" {
		compartment = b.instance.CompartmentID
	}

	deadline := time.Now().Add(o.AssignTimeout)
	for {
		fresh, err := o.client.CreateEphemeralPublicIP(ctx, compartment, b.privateIP.ID, name)
		if err == nil {
			return fresh, nil
		}
		if !ociclient.IsConflict(err) || time.Now().After(deadline) {
			return nil, err
		}
		o.logf("allocation conflicted, retrying...")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(o.PollInterval):
		}
	}
}

// waitReleased blocks until the private IP has no public IP attached.
func (o *Oracle) waitReleased(ctx context.Context, privateIPOCID string) error {
	deadline := time.Now().Add(o.ReleaseTimeout)
	for {
		_, err := o.client.GetPublicIPByPrivateIPID(ctx, privateIPOCID)
		if ociclient.IsNotFound(err) {
			return nil
		}
		if err != nil && !ociclient.IsNotFound(err) {
			// Transient lookup failures are not fatal while we are polling.
			o.logf("waiting for release: %v", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for the old public IP to detach", o.ReleaseTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.PollInterval):
		}
	}
}

// waitAssigned blocks until a public IP reaches ASSIGNED and returns its address.
func (o *Oracle) waitAssigned(ctx context.Context, publicIPOCID string) (string, error) {
	deadline := time.Now().Add(o.AssignTimeout)
	for {
		ip, err := o.client.GetPublicIP(ctx, publicIPOCID)
		if err == nil {
			if ip.LifecycleState == ociclient.PublicIPAssigned && ip.IPAddress != "" {
				return ip.IPAddress, nil
			}
			if ip.LifecycleState == ociclient.PublicIPTerminated {
				return "", errors.New("newly allocated public IP was terminated before it attached")
			}
		} else {
			o.logf("waiting for assignment: %v", err)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for the new public IP to be assigned", o.AssignTimeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(o.PollInterval):
		}
	}
}
