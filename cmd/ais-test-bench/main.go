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

const defaultPort = 8080

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
	port, err := parsePort(args, os.Stderr)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", port, err)
	}
	return app.Run(ctx, logger, ln)
}

// parsePort parses the command-line arguments. The only flag is -port, the
// HTTP port on localhost. It defaults to defaultPort and must be 1-65535.
// Usage and flag errors are printed to output.
func parsePort(args []string, output io.Writer) (int, error) {
	fs := flag.NewFlagSet("ais-test-bench", flag.ContinueOnError)
	fs.SetOutput(output)
	port := fs.Int("port", defaultPort, "HTTP port to listen on (localhost)")
	if err := fs.Parse(args); err != nil {
		return 0, fmt.Errorf("parse arguments: %w", err)
	}
	if fs.NArg() > 0 {
		return 0, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if *port < 1 || *port > 65535 {
		return 0, fmt.Errorf("port %d out of range 1-65535", *port)
	}
	return *port, nil
}
