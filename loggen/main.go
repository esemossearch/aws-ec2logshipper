// loggen is a small utility that generates configurable volumes of log output for testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main parses flags and emits log lines until interrupted or the requested count is reached.
func main() {
	var (
		rate    = flag.Float64("rate", 1.0, "lines per second")
		count   = flag.Int("count", 0, "total lines to generate (0 = infinite)")
		burst   = flag.Int("burst", 1, "lines emitted per tick")
		output  = flag.String("output", "", "output file (empty = stdout)")
		host    = flag.String("host", "test-host", "hostname in log line")
		app     = flag.String("app", "loggen", "application name in log line")
		message = flag.String("message", "generated log line", "message prefix")
	)
	flag.Parse()

	w := os.Stdout
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open output: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	interval := time.Duration(float64(time.Second) / *rate)
	if interval <= 0 {
		interval = time.Second
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	generated := 0
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-ticker.C:
			for i := 0; i < *burst; i++ {
				if *count > 0 && generated >= *count {
					break loop
				}
				ts := time.Now().Format("Jan 02 15:04:05")
				fmt.Fprintf(w, "%s %s %s: %s #%d\n", ts, *host, *app, *message, generated)
				generated++
			}
		}
	}

	fmt.Fprintf(os.Stderr, "generated %d lines\n", generated)
}
