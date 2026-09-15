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
	"strings"
	"syscall"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/cli"
	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
)

const (
	defaultAddr         = "localhost:8081"
	defaultSimulatorURL = "http://localhost:8000"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stderr, logger)
	stop()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		logger.Error("display failed", "error", err)
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

// serve runs the standalone display on ln. Serve closes the client's owned idle
// connections when it stops.
func serve(ctx context.Context, ln net.Listener, cfg config, logger *slog.Logger) error {
	return display.Serve(ctx, ln, display.StandaloneConfig{Client: cfg.client, Logger: logger, ManagerURL: cfg.managerURL, BasePath: cfg.basePath})
}

// config is the validated command-line configuration.
type config struct {
	addr       string
	client     *display.Client
	managerURL string
	basePath   string
}

// parseArgs parses the command-line arguments before any listener is bound.
// -addr is the HTTP listen address as host:port, validated by
// cli.ValidateListenAddr. -simulator-url is the standalone simulator's base URL,
// an origin with an optional path prefix. The display reads its API below
// {url}/api/, validated by display.NewClient, and links {url}/manager.
// -base-path is the display's own public route prefix, empty for the root. Usage
// and flag errors are printed to output.
func parseArgs(args []string, output io.Writer) (config, error) {
	fs := flag.NewFlagSet("display", flag.ContinueOnError)
	fs.SetOutput(output)
	addr := fs.String("addr", defaultAddr, "HTTP listen address as host:port; host is required")
	simulatorURL := fs.String("simulator-url", defaultSimulatorURL, "simulator base URL as http(s)://host[:port][/prefix]")
	basePath := fs.String("base-path", "", "public path prefix of every display route, such as /tools/ais; empty serves at the root")
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
	base := strings.TrimSuffix(*simulatorURL, "/")
	client, err := display.NewClient(base + "/api/")
	if err != nil {
		return config{}, fmt.Errorf("-simulator-url: %w", err)
	}
	return config{addr: *addr, client: client, managerURL: base + "/manager", basePath: *basePath}, nil
}
