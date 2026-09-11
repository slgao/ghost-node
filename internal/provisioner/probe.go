package provisioner

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

// ProbeResult records whether a freshly allocated address is reachable.
//
// This is a reachability check, not a censorship verdict: a TCP handshake that
// completes proves the address is routable from wherever ghostctl runs. Run it
// from inside the network you are trying to escape and that distinction mostly
// collapses — an address null-routed by a censor times out here, which is
// exactly the signal we want before committing to a new IP.
type ProbeResult struct {
	Attempted bool
	OK        bool
	Latency   time.Duration
	Err       error
}

func (p ProbeResult) String() string {
	switch {
	case !p.Attempted:
		return "skipped"
	case p.OK:
		return fmt.Sprintf("reachable in %dms", p.Latency.Milliseconds())
	default:
		return fmt.Sprintf("unreachable (%v)", p.Err)
	}
}

// ProbeTCP dials host:port up to attempts times, returning on the first success.
func ProbeTCP(ctx context.Context, host string, port int, timeout time.Duration, attempts int) ProbeResult {
	if attempts < 1 {
		attempts = 1
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	result := ProbeResult{Attempted: true}

	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				result.Err = ctx.Err()
				return result
			case <-time.After(2 * time.Second):
			}
		}

		dialer := net.Dialer{Timeout: timeout}
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			result.Err = err
			continue
		}
		conn.Close()

		result.OK = true
		result.Latency = time.Since(start)
		result.Err = nil
		return result
	}
	return result
}
