// aws-ec2logshipper tails a local log file and forwards each line to AWS CloudWatch Logs.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

func main() {
	configPath := flag.String("config", "/etc/aws-ec2logshipper/config.json", "path to JSON config")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		slog.Info("shutting down")
		cancel()
	}()

	stream := cfg.LogStream
	if stream == "" {
		id, err := getInstanceID(ctx)
		if err != nil {
			slog.Error("get instance id", "error", err)
			os.Exit(1)
		}
		stream = id
	}

	cw, err := NewCWClient(ctx, cfg.Region, cfg.LogGroup, stream, cfg.CreateLogGroup, cfg.CreateLogStream)
	if err != nil {
		slog.Error("cloudwatch client", "error", err)
		os.Exit(1)
	}
	if err := cw.EnsureGroupAndStream(ctx); err != nil {
		slog.Error("ensure log group/stream", "error", err)
		os.Exit(1)
	}

	parser, err := NewParser(cfg.TimestampLayout, cfg.TimestampRegex)
	if err != nil {
		slog.Error("parser", "error", err)
		os.Exit(1)
	}

	lines := make(chan string, 1000)
	tailer := NewTailer(cfg.LogFile, lines)

	go func() {
		if err := tailer.Run(ctx); err != nil {
			slog.Error("tailer", "error", err)
			cancel()
		}
	}()

	flush := time.Duration(cfg.FlushIntervalMs) * time.Millisecond
	ticker := time.NewTicker(flush)
	defer ticker.Stop()

	var batch []types.InputLogEvent
	for {
		select {
		case <-ctx.Done():
			if len(batch) > 0 {
				_ = cw.Put(context.Background(), batch)
			}
			return
		case <-ticker.C:
			if len(batch) > 0 {
				if err := cw.Put(ctx, batch); err != nil {
					slog.Error("put log events", "error", err)
				}
				batch = nil
			}
		case line, ok := <-lines:
			if !ok {
				return
			}
			ts, msg := parser.Parse(line)
			batch = append(batch, types.InputLogEvent{
				Message:   aws.String(msg),
				Timestamp: aws.Int64(ts.UnixMilli()),
			})
			if len(batch) >= cfg.BatchMaxSize {
				if err := cw.Put(ctx, batch); err != nil {
					slog.Error("put log events", "error", err)
				}
				batch = nil
			}
		}
	}
}
