package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// getInstanceID retrieves the EC2 instance ID using IMDSv2.
func getInstanceID(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return "", err
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "300")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("imds token request: %w", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imds token status %d", tokenResp.StatusCode)
	}

	token, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/meta-data/instance-id", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token", strings.TrimSpace(string(token)))

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("imds instance-id request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imds instance-id status %d", resp.StatusCode)
	}

	id, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(id)), nil
}
