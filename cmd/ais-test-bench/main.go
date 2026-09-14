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
	"strconv"
	"syscall"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/app"
)

const defaultAddr = "localhost:8000"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logger.Error("ais-test-bench failed", "error", err)
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
// HTTP listen address as host:port. It defaults to defaultAddr. The host is
// required: an empty host binds all interfaces, which triggers firewall
// prompts on Windows. The port must be 1-65535. Usage and flag errors are
// printed to output.
func parseAddr(args []string, output io.Writer) (string, error) {
	fs := flag.NewFlagSet("ais-test-bench", flag.ContinueOnError)
	fs.SetOutput(output)
	addr := fs.String("addr", defaultAddr, "HTTP listen address as host:port; host is required")
	if err := fs.Parse(args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	host, portText, err := net.SplitHostPort(*addr)
	if err != nil {
		return "", fmt.Errorf("invalid -addr %q: %w", *addr, err)
	}
	if host == "" {
		return "", fmt.Errorf("invalid -addr %q: host is required, e.g. %s", *addr, defaultAddr)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", fmt.Errorf("invalid -addr %q: port: %w", *addr, err)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid -addr %q: port %d out of range 1-65535", *addr, port)
	}
	return *addr, nil
}
