// Command ghostctl rotates the public IP of Ghost Node VPN servers.
//
// When a node's address is blocked, releasing its ephemeral public IP and
// drawing a new one from the cloud provider's regional pool restores service in
// well under a minute, at no cost, without touching the instance: the VPN
// process listens on 0.0.0.0 and never learns the address changed.
//
//	ghostctl status              # what address is each node on?
//	ghostctl rotate jp1          # draw a new address, verify it, update DNS
//	ghostctl rotate all --yes
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/vpnplatform/core/internal/provisioner"
	"github.com/vpnplatform/core/internal/provisioner/dns"
	"github.com/vpnplatform/core/internal/provisioner/ociclient"
)

const version = "0.1.0"

const (
	colGreen  = "\033[0;32m"
	colYellow = "\033[1;33m"
	colCyan   = "\033[0;36m"
	colRed    = "\033[0;31m"
	colBold   = "\033[1m"
	colReset  = "\033[0m"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "\ninterrupted")
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "%s%s%s\n", colRed, err, colReset)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "rotate":
		return cmdRotate(ctx, args[1:])
	case "status":
		return cmdStatus(ctx, args[1:])
	case "nodes":
		return cmdNodes(args[1:])
	case "burned":
		return cmdBurned(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("ghostctl %s\n", version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// parseFlags parses args allowing flags to appear after positional arguments,
// which the standard flag package stops at. `ghostctl rotate jp1 --yes` is the
// form people actually type.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}

func usage() {
	fmt.Printf(`%sghostctl%s %s — rotate the public IP of Ghost Node VPN servers

%sCommands%s
  rotate <node|all>   Release the current public IP and draw a new one
  status [node]       Show each node's instance state and current address
  nodes               List configured nodes
  burned              List addresses recorded as blocked
  version             Print the version

%sRotate flags%s
  --config PATH       Config file (default ~/.ghostctl/config.yaml)
  --attempts N        Addresses to draw before settling (default from config)
  --no-probe          Skip reachability verification of the new address
  --no-dns            Do not update DNS
  --no-cp             Do not notify the control plane
  --no-ssh            Do not rewrite ~/.ssh/config
  --dry-run           Show what would happen, change nothing
  -y, --yes           Do not ask for confirmation

%sExample%s
  ghostctl rotate jp1
`, colBold, colReset, version, colBold, colReset, colBold, colReset, colBold, colReset)
}

// buildProvider constructs the Oracle provider from config, preferring explicit
// credentials and falling back to an existing OCI CLI config file.
func buildProvider(cfg *Config, verbose bool) (*provisioner.Oracle, error) {
	oc := ociclient.Config{
		TenancyOCID:    cfg.Oracle.TenancyOCID,
		UserOCID:       cfg.Oracle.UserOCID,
		Fingerprint:    cfg.Oracle.Fingerprint,
		Region:         cfg.Oracle.Region,
		PrivateKeyPath: cfg.Oracle.PrivateKeyPath,
	}

	if oc.TenancyOCID == "" || oc.UserOCID == "" || oc.Fingerprint == "" {
		fromFile, err := ociclient.LoadFromFile(cfg.Oracle.ConfigFile, cfg.Oracle.Profile)
		if err != nil {
			return nil, fmt.Errorf("%w\n\nSet oracle.tenancy_ocid/user_ocid/fingerprint in the ghostctl config, "+
				"or run `oci setup config` to create ~/.oci/config", err)
		}
		// Explicit config values still win over the file.
		if oc.TenancyOCID == "" {
			oc.TenancyOCID = fromFile.TenancyOCID
		}
		if oc.UserOCID == "" {
			oc.UserOCID = fromFile.UserOCID
		}
		if oc.Fingerprint == "" {
			oc.Fingerprint = fromFile.Fingerprint
		}
		if oc.Region == "" {
			oc.Region = fromFile.Region
		}
		if oc.PrivateKeyPath == "" {
			oc.PrivateKeyPath = fromFile.PrivateKeyPath
		}
	}

	client, err := ociclient.New(oc)
	if err != nil {
		return nil, err
	}

	oracle := provisioner.NewOracle(client)
	if verbose {
		oracle.Logf = func(format string, args ...any) {
			fmt.Printf("  %s→%s %s\n", colCyan, colReset, fmt.Sprintf(format, args...))
		}
	}
	return oracle, nil
}

func targetsFor(cfg *Config, selector string) ([]provisioner.Target, error) {
	var picked []NodeConfig
	if selector == "all" {
		picked = cfg.Nodes
	} else {
		n, ok := cfg.Node(selector)
		if !ok {
			names := make([]string, 0, len(cfg.Nodes))
			for _, c := range cfg.Nodes {
				names = append(names, c.Name)
			}
			return nil, fmt.Errorf("no node named %q — configured nodes: %s", selector, strings.Join(names, ", "))
		}
		picked = []NodeConfig{n}
	}

	targets := make([]provisioner.Target, 0, len(picked))
	for _, n := range picked {
		targets = append(targets, provisioner.Target{
			Name:       n.Name,
			InstanceID: n.InstanceOCID,
			DNSRecord:  n.DNSRecord,
			NodeID:     n.NodeID,
			SSHHost:    n.SSHHost,
			ProbePort:  n.ProbePort,
		})
	}
	return targets, nil
}

func cmdRotate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("rotate", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	attempts := fs.Int("attempts", 0, "addresses to draw before settling")
	noProbe := fs.Bool("no-probe", false, "skip verification of the new address")
	noDNS := fs.Bool("no-dns", false, "do not update DNS")
	noCP := fs.Bool("no-cp", false, "do not notify the control plane")
	noSSH := fs.Bool("no-ssh", false, "do not rewrite ~/.ssh/config")
	dryRun := fs.Bool("dry-run", false, "show what would happen, change nothing")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	fs.BoolVar(yes, "y", false, "do not ask for confirmation")
	positional, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("usage: ghostctl rotate <node|all>")
	}

	cfg, path, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	targets, err := targetsFor(cfg, positional[0])
	if err != nil {
		return err
	}

	oracle, err := buildProvider(cfg, true)
	if err != nil {
		return err
	}

	fmt.Printf("%sconfig%s  %s\n", colBold, colReset, path)
	fmt.Printf("%sprovider%s %s\n\n", colBold, colReset, oracle.Name())

	if *dryRun {
		return dryRunRotate(ctx, oracle, cfg, targets)
	}

	burned, err := provisioner.LoadBurnedStore(cfg.Rotate.BurnedStore)
	if err != nil {
		return err
	}

	rotator := provisioner.NewRotator(oracle)
	rotator.Burned = burned
	rotator.Attempts = cfg.Rotate.Attempts
	rotator.Probe = cfg.Rotate.Probe && !*noProbe
	rotator.ProbeTimeout = cfg.Rotate.ProbeTimeout
	rotator.ProbeAttempts = cfg.Rotate.ProbeAttempts
	rotator.BurnWindow = cfg.Rotate.BurnWindow
	rotator.Logf = func(format string, args ...any) {
		fmt.Printf("  %s\n", fmt.Sprintf(format, args...))
	}
	if *attempts > 0 {
		rotator.Attempts = *attempts
	}
	rotator.Post = buildPostActions(cfg, *noDNS, *noCP, *noSSH)

	if !*yes {
		ok, err := confirm(ctx, oracle, targets)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	var failures int
	for _, target := range targets {
		fmt.Printf("\n%s%s%s\n", colBold, target.Name, colReset)

		result, err := rotator.Rotate(ctx, target)
		if err != nil {
			failures++
			fmt.Printf("  %sfailed:%s %v\n", colRed, colReset, err)
			continue
		}
		printResult(result)
	}

	if err := burned.Save(cfg.Rotate.BurnWindow); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s %v\n", colYellow, colReset, err)
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d rotations failed", failures, len(targets))
	}
	return nil
}

func buildPostActions(cfg *Config, noDNS, noCP, noSSH bool) []provisioner.PostAction {
	var actions []provisioner.PostAction

	if !noDNS && cfg.Cloudflare.APIToken != "" {
		cf := dns.NewCloudflare(cfg.Cloudflare.APIToken, cfg.Cloudflare.ZoneID)
		cf.TTL = cfg.Cloudflare.TTL
		actions = append(actions, provisioner.DNSAction{Updater: cf})
	}
	if !noCP && cfg.ControlPlane.URL != "" {
		actions = append(actions, provisioner.ControlPlaneAction{
			BaseURL:    cfg.ControlPlane.URL,
			AdminToken: cfg.ControlPlane.AdminToken,
		})
	}
	if !noSSH {
		actions = append(actions, provisioner.SSHConfigAction{})
	}
	return actions
}

func printResult(r *provisioner.Result) {
	old := r.OldIP
	if old == "" {
		old = "(none)"
	}
	fmt.Printf("  %s%s → %s%s", colGreen, old, r.NewIP, colReset)
	if r.Attempts > 1 {
		fmt.Printf("  (%d draws)", r.Attempts)
	}
	fmt.Println()

	if r.Probe.Attempted {
		marker, colour := "ok", colGreen
		if !r.Probe.OK {
			marker, colour = "warning", colYellow
		}
		fmt.Printf("  %s%s:%s %s\n", colour, marker, colReset, r.Probe)
	}
	for _, rejected := range r.Rejected {
		fmt.Printf("  %sdiscarded:%s %s\n", colYellow, colReset, rejected)
	}
	for name, err := range r.PostErrors {
		fmt.Printf("  %s%s failed:%s %v\n", colRed, name, colReset, err)
	}
	if r.Target.DNSRecord != "" {
		fmt.Printf("  %s now resolves to %s\n", r.Target.DNSRecord, r.NewIP)
	}
}

func dryRunRotate(ctx context.Context, oracle *provisioner.Oracle, cfg *Config, targets []provisioner.Target) error {
	fmt.Printf("%sdry run — nothing will change%s\n\n", colYellow, colReset)

	for _, target := range targets {
		name, state, ip, lifetime, err := oracle.Describe(ctx, target.InstanceID)
		if err != nil {
			fmt.Printf("%s%s%s\n  %serror:%s %v\n\n", colBold, target.Name, colReset, colRed, colReset, err)
			continue
		}

		fmt.Printf("%s%s%s (%s, %s)\n", colBold, target.Name, colReset, name, strings.ToLower(state))
		fmt.Printf("  current address   %s (%s)\n", ip, strings.ToLower(lifetime))
		if lifetime == "RESERVED" {
			fmt.Printf("  %swould fail:%s %v\n", colRed, colReset, provisioner.ErrReservedIP)
		} else {
			fmt.Printf("  would release %s and draw a new address (up to %d times)\n", ip, cfg.Rotate.Attempts)
		}
		if target.DNSRecord != "" && cfg.Cloudflare.APIToken != "" {
			fmt.Printf("  would repoint     %s\n", target.DNSRecord)
		}
		if target.NodeID != "" && cfg.ControlPlane.URL != "" {
			fmt.Printf("  would update      control plane node %s\n", target.NodeID)
		}
		if target.SSHHost != "" {
			fmt.Printf("  would update      ssh config Host %s\n", target.SSHHost)
		}
		fmt.Println()
	}
	return nil
}

func confirm(ctx context.Context, oracle *provisioner.Oracle, targets []provisioner.Target) (bool, error) {
	fmt.Printf("%sAbout to rotate:%s\n", colBold, colReset)
	for _, t := range targets {
		ip, err := oracle.CurrentIP(ctx, t.InstanceID)
		switch {
		case errors.Is(err, provisioner.ErrNoPublicIP):
			ip = "(no public IP)"
		case err != nil:
			return false, fmt.Errorf("reading current address of %s: %w", t.Name, err)
		}
		fmt.Printf("  %-12s %s\n", t.Name, ip)
	}
	fmt.Printf("\nExisting connections to these addresses will drop. Continue? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	positional, err := parseFlags(fs, args)
	if err != nil {
		return err
	}

	cfg, _, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}

	selector := "all"
	if len(positional) == 1 {
		selector = positional[0]
	} else if len(positional) > 1 {
		return errors.New("usage: ghostctl status [node]")
	}
	targets, err := targetsFor(cfg, selector)
	if err != nil {
		return err
	}

	oracle, err := buildProvider(cfg, false)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NODE\tINSTANCE\tSTATE\tADDRESS\tLIFETIME\tREACHABLE")

	for _, t := range targets {
		name, state, ip, lifetime, err := oracle.Describe(ctx, t.InstanceID)
		if err != nil {
			fmt.Fprintf(w, "%s\t-\terror\t-\t-\t%s%v%s\n", t.Name, colRed, err, colReset)
			continue
		}
		if ip == "" {
			ip, lifetime = "(none)", "-"
		}

		reach := "-"
		if ip != "(none)" {
			probe := provisioner.ProbeTCP(ctx, ip, portOr443(t.ProbePort), 5*time.Second, 1)
			if probe.OK {
				reach = fmt.Sprintf("%s%dms%s", colGreen, probe.Latency.Milliseconds(), colReset)
			} else {
				reach = colRed + "no" + colReset
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", t.Name, name, strings.ToLower(state), ip, strings.ToLower(lifetime), reach)
	}
	return w.Flush()
}

func portOr443(p int) int {
	if p > 0 {
		return p
	}
	return 443
}

func cmdNodes(args []string) error {
	fs := flag.NewFlagSet("nodes", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}

	cfg, path, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	fmt.Printf("%s%s%s\n\n", colBold, path, colReset)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NODE\tDNS RECORD\tSSH HOST\tPORT\tCONTROL PLANE ID")
	for _, n := range cfg.Nodes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			n.Name, orDash(n.DNSRecord), orDash(n.SSHHost), portOr443(n.ProbePort), orDash(n.NodeID))
	}
	return w.Flush()
}

func cmdBurned(args []string) error {
	fs := flag.NewFlagSet("burned", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file path")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}

	cfg, _, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	store, err := provisioner.LoadBurnedStore(cfg.Rotate.BurnedStore)
	if err != nil {
		return err
	}

	ips := store.List()
	if len(ips) == 0 {
		fmt.Println("no burned addresses recorded")
		return nil
	}
	fmt.Printf("%s%d burned addresses%s (avoided for %s)\n\n", colBold, len(ips), colReset, cfg.Rotate.BurnWindow)
	for _, ip := range ips {
		fmt.Printf("  %s\n", ip)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
