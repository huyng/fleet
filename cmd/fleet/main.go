// Command fleet monitors a group of computers and controls them.
//
// It runs in one of two roles, chosen by subcommand:
//
//	fleet master -config master.toml   collect reports, serve the web page
//	fleet node -config node.toml       report to the master, run its commands
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/huyng/fleet/internal/agent"
	"github.com/huyng/fleet/internal/config"
	"github.com/huyng/fleet/internal/master"
	"github.com/huyng/fleet/internal/version"
)

// usage is printed when the command line cannot be understood.
const usage = `fleet monitors and controls a group of computers.

usage:
  fleet master -config <file>   run the collector and web page
  fleet node -config <file>     report this machine to a master
  fleet version                 print the version
`

func main() {
	// Ctrl-C and SIGTERM stop the role cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "fleet: %v\n", err)
		os.Exit(1)
	}
}

// run dispatches to the subcommand named by args.
//
// Args:
//
//	ctx: cancelled to stop the subcommand.
//	args: command line arguments, without the program name.
//	stderr: where usage and subcommand help is written.
//
// Raises:
//
//	flag.ErrHelp when help was asked for, and any error the subcommand returns.
func run(ctx context.Context, args []string, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("no subcommand given")
	}

	switch args[0] {
	case "master":
		return runMaster(ctx, args[1:], stderr)
	case "node":
		return runNode(ctx, args[1:], stderr)
	case "version":
		fmt.Println(version.Version)
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(stderr, usage)
		return nil
	default:
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

// runMaster loads the master config and serves until ctx is cancelled.
func runMaster(ctx context.Context, args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("master", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to the master TOML config")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("master needs -config")
	}

	cfg, err := config.LoadMaster(*configPath)
	if err != nil {
		return err
	}
	return master.Serve(ctx, cfg)
}

// runNode loads the node config and reports to the master until ctx is
// cancelled.
func runNode(ctx context.Context, args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("node", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to the node TOML config")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("node needs -config")
	}

	cfg, err := config.LoadNode(*configPath)
	if err != nil {
		return err
	}
	return agent.Run(ctx, cfg)
}
