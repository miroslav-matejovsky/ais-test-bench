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

	"github.com/miroslav-matejovsky/ais-testbench/internal/app"
	"github.com/miroslav-matejovsky/ais-testbench/internal/cli"
)

const defaultAddr = "localhost:8000"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logger.Error("ais-testbench failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	addr, err := parseAddr(args, os.Stderr)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	return app.Run(ctx, logger, ln)
}

// parseAddr parses the command-line arguments. The only flag is -addr, the
// public HTTP listen address as host:port, validated by cli.ValidateListenAddr.
// It defaults to defaultAddr. Usage and flag errors are printed to output.
func parseAddr(args []string, output io.Writer) (string, error) {
	fs := flag.NewFlagSet("ais-testbench", flag.ContinueOnError)
	fs.SetOutput(output)
	addr := fs.String("addr", defaultAddr, "HTTP listen address as host:port; host is required")
	if err := fs.Parse(args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if err := cli.ValidateListenAddr(*addr); err != nil {
		return "", fmt.Errorf("-addr: %w", err)
	}
	return *addr, nil
}
