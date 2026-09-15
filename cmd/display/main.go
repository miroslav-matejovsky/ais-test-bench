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
)

const (
	defaultAddr         = "localhost:8081"
	defaultSimulatorURL = "http://localhost:8000"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logger.Error("display failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	cfg, err := parseArgs(args, os.Stderr)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.addr, err)
	}
	return display.Run(ctx, ln, display.StandaloneConfig{Client: cfg.client, Logger: logger, ManagerURL: cfg.managerURL})
}

// config is the validated command-line configuration.
type config struct {
	addr       string
	client     *display.Client
	managerURL string
}

// parseArgs parses the command-line arguments before any listener is bound.
// -addr is the HTTP listen address as host:port, validated by
// cli.ValidateListenAddr. -simulator-url is the standalone simulator's base URL,
// an origin with an optional path prefix. The display reads its API below
// {url}/api/, validated by display.NewClient, and links {url}/manager. Usage and
// flag errors are printed to output.
func parseArgs(args []string, output io.Writer) (config, error) {
	fs := flag.NewFlagSet("display", flag.ContinueOnError)
	fs.SetOutput(output)
	addr := fs.String("addr", defaultAddr, "HTTP listen address as host:port; host is required")
	simulatorURL := fs.String("simulator-url", defaultSimulatorURL, "simulator base URL as http(s)://host[:port][/prefix]")
	if err := fs.Parse(args); err != nil {
		return config{}, fmt.Errorf("parse arguments: %w", err)
	}
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if err := cli.ValidateListenAddr(*addr); err != nil {
		return config{}, fmt.Errorf("-addr: %w", err)
	}
	base := strings.TrimSuffix(*simulatorURL, "/")
	client, err := display.NewClient(base + "/api/")
	if err != nil {
		return config{}, fmt.Errorf("-simulator-url: %w", err)
	}
	return config{addr: *addr, client: client, managerURL: base + "/manager"}, nil
}
