package provisioner

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Target is one rotatable node.
type Target struct {
	// Name is the short label used on the command line.
	Name string
	// InstanceID is the cloud instance identifier (an OCID on Oracle).
	InstanceID string
	// DNSRecord, when set, is the FQDN repointed at the new address.
	DNSRecord string
	// NodeID is the control-plane node UUID, for syncing the new address.
	NodeID string
	// ProbePort is the port verified after rotation. Defaults to 443.
	ProbePort int
	// SSHHost is the ~/.ssh/config Host alias to keep pointed at this node.
	SSHHost string
}

func (t Target) probePort() int {
	if t.ProbePort > 0 {
		return t.ProbePort
	}
	return 443
}

// PostAction runs after a new address has been accepted. Post actions are
// best-effort: the address has already changed by the time they run, so a
// failure is reported but does not fail the rotation.
type PostAction interface {
	Name() string
	Apply(ctx context.Context, target Target, oldIP, newIP string) error
}

// Result describes what a rotation did.
type Result struct {
	Target   Target
	OldIP    string
	NewIP    string
	Attempts int
	// Rejected lists addresses drawn and discarded, with the reason.
	Rejected []string
	Probe    ProbeResult
	// PostErrors holds failures from post actions, keyed by action name.
	PostErrors map[string]error
}

// Rotator draws a new public address for a node and verifies it before
// committing to it.
//
// A fresh address is unverified: Oracle's regional pool contains plenty of
// addresses that are already blocked, so drawing one and advertising it
// immediately can swap a dead IP for another dead IP. Rotation is free and
// takes under a minute, so the loop here simply draws again.
type Rotator struct {
	Provider Provider
	Burned   *BurnedStore
	Post     []PostAction

	// Attempts caps how many addresses are drawn before giving up.
	Attempts int
	// Probe enables post-rotation reachability verification.
	Probe bool
	// ProbeTimeout bounds a single dial.
	ProbeTimeout time.Duration
	// ProbeAttempts is how many dials count as a fair test.
	ProbeAttempts int
	// BurnWindow is how long a failed address is avoided for.
	BurnWindow time.Duration

	Logf func(format string, args ...any)
}

func NewRotator(p Provider) *Rotator {
	return &Rotator{
		Provider:      p,
		Attempts:      3,
		Probe:         true,
		ProbeTimeout:  6 * time.Second,
		ProbeAttempts: 2,
		BurnWindow:    30 * 24 * time.Hour,
	}
}

func (r *Rotator) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

// Rotate draws addresses until one verifies, then runs the post actions.
func (r *Rotator) Rotate(ctx context.Context, target Target) (*Result, error) {
	if r.Provider == nil {
		return nil, errors.New("rotator: no provider configured")
	}
	attempts := r.Attempts
	if attempts < 1 {
		attempts = 1
	}

	result := &Result{Target: target, PostErrors: map[string]error{}}

	for attempt := 1; attempt <= attempts; attempt++ {
		result.Attempts = attempt
		r.logf("[%s] attempt %d/%d: rotating public IP", target.Name, attempt, attempts)

		oldIP, newIP, err := r.Provider.RotateIP(ctx, target.InstanceID)
		if oldIP != "" && result.OldIP == "" {
			result.OldIP = oldIP
		}
		if err != nil {
			return result, fmt.Errorf("rotating %s: %w", target.Name, err)
		}
		result.NewIP = newIP

		last := attempt == attempts

		// A neighbour of a recently burned address is a poor bet — unless this
		// is the final attempt, in which case a working-unknown address still
		// beats the one we just released.
		if r.Burned != nil && !last {
			if burned, why := r.Burned.Burned(newIP, r.BurnWindow); burned {
				r.logf("[%s] %s rejected: %s", target.Name, newIP, why)
				result.Rejected = append(result.Rejected, fmt.Sprintf("%s (%s)", newIP, why))
				continue
			}
		}

		if !r.Probe {
			result.Probe = ProbeResult{}
			break
		}

		probe := ProbeTCP(ctx, newIP, target.probePort(), r.ProbeTimeout, r.ProbeAttempts)
		result.Probe = probe
		if probe.OK {
			r.logf("[%s] %s verified: %s", target.Name, newIP, probe)
			break
		}

		r.logf("[%s] %s failed verification: %s", target.Name, newIP, probe)
		if r.Burned != nil {
			r.Burned.Add(newIP)
		}
		if last {
			result.Rejected = append(result.Rejected, fmt.Sprintf("%s (unverified, kept anyway)", newIP))
			break
		}
		result.Rejected = append(result.Rejected, fmt.Sprintf("%s (%v)", newIP, probe.Err))
	}

	// The released address is burned by definition — it is the one we fled.
	if r.Burned != nil && result.OldIP != "" {
		r.Burned.Add(result.OldIP)
	}

	for _, action := range r.Post {
		if err := action.Apply(ctx, target, result.OldIP, result.NewIP); err != nil {
			r.logf("[%s] %s: %v", target.Name, action.Name(), err)
			result.PostErrors[action.Name()] = err
			continue
		}
		r.logf("[%s] %s updated", target.Name, action.Name())
	}

	return result, nil
}
