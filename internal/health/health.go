package health

import (
	"context"
	"fmt"
	"strings"
	"time"

	sshrunner "github.com/ikox01/upcloud-box/internal/ssh"
)

type HTTPCheckRequest struct {
	Host     string
	URL      string
	Timeout  time.Duration
	Interval time.Duration
}

func WaitHTTPReady(ctx context.Context, runner *sshrunner.Runner, req HTTPCheckRequest) error {
	if runner == nil {
		return fmt.Errorf("ssh runner is required")
	}

	host := strings.TrimSpace(req.Host)
	if host == "" {
		return fmt.Errorf("health check host is required")
	}

	url := strings.TrimSpace(req.URL)
	if url == "" {
		return fmt.Errorf("health check url is required")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	interval := req.Interval
	if interval <= 0 {
		interval = 3 * time.Second
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := fmt.Sprintf("curl -fsS --max-time 5 %s >/dev/null", shellQuote(url))

	for {
		_, err := runner.Run(checkCtx, host, cmd)
		if err == nil {
			return nil
		}

		if checkCtx.Err() != nil {
			return fmt.Errorf("health check timeout after %s for %s: %w", timeout, url, err)
		}

		timer := time.NewTimer(interval)
		select {
		case <-checkCtx.Done():
			timer.Stop()
			return fmt.Errorf("health check timeout after %s for %s", timeout, url)
		case <-timer.C:
		}
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
