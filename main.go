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

	slog.Info("starting aws-ec2logshipper", "config_path", *configPath)
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	slog.Info("configuration loaded",
		"log_file", cfg.LogFile,
		"log_file_mode", cfg.LogFileMode,
		"log_group", cfg.LogGroup,
		"region", cfg.Region,
		"timestamp_layout", cfg.TimestampLayout,
		"timestamp_regex", cfg.TimestampRegex,
		"batch_max_size", cfg.BatchMaxSize,
		"flush_interval_ms", cfg.FlushIntervalMs,
		"create_log_group", cfg.CreateLogGroup,
		"create_log_stream", cfg.CreateLogStream,
	)

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
		slog.Info("log stream is empty; resolving EC2 instance ID via IMDSv2")
		id, err := getInstanceID(ctx)
		if err != nil {
			slog.Error("get instance id", "error", err)
			os.Exit(1)
		}
		stream = id
		slog.Info("using EC2 instance ID as log stream", "log_stream", stream)
	} else {
		slog.Info("using configured log stream", "log_stream", stream)
	}

	slog.Info("configuring CloudWatch Logs client", "region", cfg.Region, "log_group", cfg.LogGroup, "log_stream", stream)
	cw, err := NewCWClient(ctx, cfg.Region, cfg.LogGroup, stream, cfg.CreateLogGroup, cfg.CreateLogStream)
	if err != nil {
		slog.Error("cloudwatch client", "error", err)
		os.Exit(1)
	}
	if err := cw.EnsureGroupAndStream(ctx); err != nil {
		slog.Error("ensure log group/stream", "error", err)
		os.Exit(1)
	}
	slog.Info("CloudWatch Logs setup completed", "create_log_group", cfg.CreateLogGroup, "create_log_stream", cfg.CreateLogStream)

	parser, err := NewParser(cfg.TimestampLayout, cfg.TimestampRegex)
	if err != nil {
		slog.Error("parser", "error", err)
		os.Exit(1)
	}
	slog.Info("timestamp parser configured", "layout", cfg.TimestampLayout, "regex", cfg.TimestampRegex)

	lines := make(chan string, 1000)
	tailer := NewTailer(cfg.LogFile, cfg.LogFileMode, lines)
	slog.Info("starting log file tailer", "pattern", cfg.LogFile, "mode", cfg.LogFileMode, "batch_max_size", cfg.BatchMaxSize, "flush_interval_ms", cfg.FlushIntervalMs)

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
