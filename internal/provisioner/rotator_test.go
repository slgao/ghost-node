package provisioner

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"
)

// fakeProvider hands out a scripted sequence of addresses.
type fakeProvider struct {
	addresses []string
	calls     int
	err       error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) CurrentIP(context.Context, string) (string, error) {
	return "203.0.113.1", nil
}

func (f *fakeProvider) RotateIP(context.Context, string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	old := "203.0.113.1"
	if f.calls > 0 {
		old = f.addresses[f.calls-1]
	}
	next := f.addresses[f.calls]
	f.calls++
	return old, next, nil
}

func newTestRotator(p Provider) *Rotator {
	r := NewRotator(p)
	r.Probe = false
	return r
}

func TestRotateRedrawsPastBurnedAddresses(t *testing.T) {
	burned, err := LoadBurnedStore("")
	if err != nil {
		t.Fatal(err)
	}
	burned.Add("198.51.100.20") // same /24 as the first draw below

	provider := &fakeProvider{addresses: []string{"198.51.100.5", "203.0.113.9"}}
	r := newTestRotator(provider)
	r.Burned = burned
	r.Attempts = 3

	result, err := r.Rotate(context.Background(), Target{Name: "jp1"})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if result.NewIP != "203.0.113.9" {
		t.Errorf("NewIP = %s, want the second draw 203.0.113.9", result.NewIP)
	}
	if result.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", result.Attempts)
	}
	if len(result.Rejected) != 1 {
		t.Errorf("Rejected = %v, want one entry", result.Rejected)
	}
}

func TestRotateBurnsTheReleasedAddress(t *testing.T) {
	burned, err := LoadBurnedStore("")
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{addresses: []string{"203.0.113.9"}}
	r := newTestRotator(provider)
	r.Burned = burned

	if _, err := r.Rotate(context.Background(), Target{Name: "jp1"}); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	// The address we fled is blocked by definition — never draw it again.
	if ok, _ := burned.Burned("203.0.113.1", time.Hour); !ok {
		t.Error("the released address should have been recorded as burned")
	}
}

func TestRotateKeepsLastDrawWhenAttemptsExhausted(t *testing.T) {
	burned, err := LoadBurnedStore("")
	if err != nil {
		t.Fatal(err)
	}
	burned.Add("198.51.100.1")

	// Every draw lands in the burned /24; a working-unknown address still
	// beats the one we already released.
	provider := &fakeProvider{addresses: []string{"198.51.100.5", "198.51.100.6"}}
	r := newTestRotator(provider)
	r.Burned = burned
	r.Attempts = 2

	result, err := r.Rotate(context.Background(), Target{Name: "jp1"})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if result.NewIP != "198.51.100.6" {
		t.Errorf("NewIP = %s, want the final draw to be kept", result.NewIP)
	}
}

func TestRotateProbeAcceptsAReachableAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	provider := &fakeProvider{addresses: []string{"127.0.0.1"}}
	r := NewRotator(provider)
	r.ProbeTimeout = 2 * time.Second
	r.ProbeAttempts = 1

	result, err := r.Rotate(context.Background(), Target{Name: "jp1", ProbePort: port})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if !result.Probe.OK {
		t.Errorf("probe should have succeeded against a live listener: %v", result.Probe.Err)
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
}

func TestRotateRedrawsWhenProbeFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// 192.0.2.0/24 is TEST-NET-1: reserved for documentation and not routable,
	// so the first draw fails its probe and the loop should draw again.
	provider := &fakeProvider{addresses: []string{"192.0.2.1", "127.0.0.1"}}
	r := NewRotator(provider)
	r.Attempts = 2
	r.ProbeTimeout = 1500 * time.Millisecond
	r.ProbeAttempts = 1

	result, err := r.Rotate(context.Background(), Target{Name: "jp1", ProbePort: port})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if result.NewIP != "127.0.0.1" || !result.Probe.OK {
		t.Errorf("expected a redraw to the reachable address, got %s (probe %v)", result.NewIP, result.Probe)
	}
}

type fakeAction struct {
	name    string
	err     error
	applied bool
	sawIP   string
}

func (f *fakeAction) Name() string { return f.name }

func (f *fakeAction) Apply(_ context.Context, _ Target, _, newIP string) error {
	f.applied, f.sawIP = true, newIP
	return f.err
}

func TestRotateRunsPostActionsAndSurvivesTheirFailures(t *testing.T) {
	ok := &fakeAction{name: "dns"}
	broken := &fakeAction{name: "control-plane", err: errors.New("connection refused")}

	provider := &fakeProvider{addresses: []string{"203.0.113.9"}}
	r := newTestRotator(provider)
	r.Post = []PostAction{broken, ok}

	result, err := r.Rotate(context.Background(), Target{Name: "jp1"})
	if err != nil {
		t.Fatalf("a failing post action must not fail the rotation: %v", err)
	}
	if !ok.applied || ok.sawIP != "203.0.113.9" {
		t.Errorf("later actions should still run; applied=%v ip=%s", ok.applied, ok.sawIP)
	}
	if result.PostErrors["control-plane"] == nil {
		t.Error("the post-action failure should be reported in the result")
	}
}

func TestRotatePropagatesProviderFailure(t *testing.T) {
	provider := &fakeProvider{err: errors.New("out of capacity")}
	r := newTestRotator(provider)

	if _, err := r.Rotate(context.Background(), Target{Name: "jp1"}); err == nil {
		t.Fatal("expected the provider error to surface")
	}
}

func TestRotateRequiresAProvider(t *testing.T) {
	r := &Rotator{}
	if _, err := r.Rotate(context.Background(), Target{Name: "jp1"}); err == nil {
		t.Fatal("expected an error when no provider is configured")
	}
}
