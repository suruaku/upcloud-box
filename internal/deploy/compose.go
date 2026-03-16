package deploy

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/suruaku/upcloud-app-platform/internal/health"
)

type ComposeRequest struct {
	Host                string
	ComposeLocalPath    string
	ComposeFileName     string
	RemoteDir           string
	HealthcheckURL      string
	HealthcheckTimeout  time.Duration
	HealthcheckInterval time.Duration
}

func (d *Deployer) RunCompose(ctx context.Context, req ComposeRequest) error {
	if err := validateComposeRequest(req); err != nil {
		return err
	}

	if _, err := d.runner.Run(ctx, req.Host, "docker compose version >/dev/null 2>&1"); err != nil {
		return fmt.Errorf("docker compose plugin is required on remote host: %w", err)
	}

	if _, err := d.runner.Run(ctx, req.Host, fmt.Sprintf("mkdir -p %s", shellQuote(req.RemoteDir))); err != nil {
		return fmt.Errorf("prepare remote compose directory %q: %w", req.RemoteDir, err)
	}

	remoteComposePath := path.Join(req.RemoteDir, req.ComposeFileName)
	if err := d.runner.UploadFile(ctx, req.Host, req.ComposeLocalPath, remoteComposePath); err != nil {
		return fmt.Errorf("upload compose file: %w", err)
	}

	if _, err := d.runner.Run(ctx, req.Host, fmt.Sprintf("cd %s && docker compose -f %s pull", shellQuote(req.RemoteDir), shellQuote(remoteComposePath))); err != nil {
		return fmt.Errorf("pull compose services: %w", err)
	}

	if _, err := d.runner.Run(ctx, req.Host, fmt.Sprintf("cd %s && docker compose -f %s up -d --remove-orphans", shellQuote(req.RemoteDir), shellQuote(remoteComposePath))); err != nil {
		return fmt.Errorf("start compose services: %w", err)
	}

	if strings.TrimSpace(req.HealthcheckURL) == "" {
		return nil
	}

	if err := health.WaitHTTPReady(ctx, d.runner, health.HTTPCheckRequest{
		Host:     req.Host,
		URL:      req.HealthcheckURL,
		Timeout:  req.HealthcheckTimeout,
		Interval: req.HealthcheckInterval,
	}); err != nil {
		return fmt.Errorf("compose services started but health check failed: %w", err)
	}

	return nil
}

func validateComposeRequest(req ComposeRequest) error {
	if strings.TrimSpace(req.Host) == "" {
		return fmt.Errorf("deploy host is required")
	}
	if strings.TrimSpace(req.ComposeLocalPath) == "" {
		return fmt.Errorf("compose local path is required")
	}
	if strings.TrimSpace(req.ComposeFileName) == "" {
		return fmt.Errorf("compose file name is required")
	}
	if strings.TrimSpace(req.RemoteDir) == "" {
		return fmt.Errorf("remote compose directory is required")
	}
	return nil
}
