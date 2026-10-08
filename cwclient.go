package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// CWClient wraps the AWS CloudWatch Logs client for a single group/stream pair.
type CWClient struct {
	svc           *cloudwatchlogs.Client
	group, stream string
	createGroup   bool
	createStream  bool
	nextToken     *string
}

// NewCWClient creates a CloudWatch Logs client configured for the given AWS region and stream.
func NewCWClient(ctx context.Context, region, group, stream string, createGroup, createStream bool) (*CWClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return &CWClient{
		svc:          cloudwatchlogs.NewFromConfig(cfg),
		group:        group,
		stream:       stream,
		createGroup:  createGroup,
		createStream: createStream,
	}, nil
}

// EnsureGroupAndStream creates the CloudWatch Logs group and stream when requested.
// Existing resources are ignored so the call is safe to repeat.
func (c *CWClient) EnsureGroupAndStream(ctx context.Context) error {
	if c.createGroup {
		_, err := c.svc.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(c.group),
		})
		if err != nil {
			var rae *types.ResourceAlreadyExistsException
			if !errors.As(err, &rae) {
				return fmt.Errorf("create log group: %w", err)
			}
		}
	}

	if c.createStream {
		_, err := c.svc.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
			LogGroupName:  aws.String(c.group),
			LogStreamName: aws.String(c.stream),
		})
		if err != nil {
			var rae *types.ResourceAlreadyExistsException
			if !errors.As(err, &rae) {
				return fmt.Errorf("create log stream: %w", err)
			}
		}
	}

	return nil
}

// Put sends log events to CloudWatch Logs, handling sequence-token retries.
func (c *CWClient) Put(ctx context.Context, events []types.InputLogEvent) error {
	if len(events) == 0 {
		return nil
	}

	input := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(c.group),
		LogStreamName: aws.String(c.stream),
		LogEvents:     events,
	}
	if c.nextToken != nil {
		input.SequenceToken = c.nextToken
	}

	out, err := c.svc.PutLogEvents(ctx, input)
	if err != nil {
		var ist *types.InvalidSequenceTokenException
		if errors.As(err, &ist) {
			c.nextToken = ist.ExpectedSequenceToken
			input.SequenceToken = c.nextToken
			out, err = c.svc.PutLogEvents(ctx, input)
		}
		if err != nil {
			return fmt.Errorf("put log events: %w", err)
		}
	}

	c.nextToken = out.NextSequenceToken
	return nil
}
