package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/miroslav-matejovsky/ais-testbench/internal/cli"
	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/testbench"
)

const defaultAddr = "localhost:8000"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stderr, logger)
	stop()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		logger.Error("ais-testbench failed", "error", err)
		os.Exit(1)
	}
}

// run parses args, binds the listener, and serves until ctx is cancelled.
func run(ctx context.Context, args []string, output io.Writer, logger *slog.Logger) error {
	cfg, err := parseArgs(args, output)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.addr, err)
	}
	return serve(ctx, ln, cfg, logger)
}

// serve runs the demonstration bench on ln through the public facade.
func serve(ctx context.Context, ln net.Listener, cfg config, logger *slog.Logger) error {
	return testbench.Serve(ctx, ln, testbench.Config{Simulation: simulator.DemoConfig(), Logger: logger, BasePath: cfg.basePath})
}

// config is the validated command-line configuration.
type config struct {
	addr     string
	basePath string
}

// parseArgs parses the command-line arguments before any listener is bound.
// -addr is the HTTP listen address as host:port, validated by
// cli.ValidateListenAddr. -base-path is the public prefix of every route, such
// as /tools/ais, and empty for the root. Usage and flag errors are printed to
// output.
func parseArgs(args []string, output io.Writer) (config, error) {
	fs := flag.NewFlagSet("ais-testbench", flag.ContinueOnError)
	fs.SetOutput(output)
	addr := fs.String("addr", defaultAddr, "HTTP listen address as host:port; host is required")
	basePath := fs.String("base-path", "", "public path prefix of every route, such as /tools/ais; empty serves at the root")
	if err := fs.Parse(args); err != nil {
		return config{}, fmt.Errorf("parse arguments: %w", err)
	}
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if err := cli.ValidateListenAddr(*addr); err != nil {
		return config{}, fmt.Errorf("-addr: %w", err)
	}
	if err := urlpath.CheckPrefix(*basePath); err != nil {
		return config{}, fmt.Errorf("-base-path: %w", err)
	}
	return config{addr: *addr, basePath: *basePath}, nil
}
