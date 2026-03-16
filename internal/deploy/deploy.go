package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/suruaku/upcloud-app-platform/internal/health"
	sshrunner "github.com/suruaku/upcloud-app-platform/internal/ssh"
)

type Request struct {
	Host                string
	ContainerName       string
	Image               string
	Port                string
	EnvFile             string
	HealthcheckURL      string
	HealthcheckTimeout  time.Duration
	HealthcheckInterval time.Duration
}

type Deployer struct {
	runner *sshrunner.Runner
}

func New(runner *sshrunner.Runner) (*Deployer, error) {
	if runner == nil {
		return nil, fmt.Errorf("ssh runner is required")
	}
	return &Deployer{runner: runner}, nil
}

func (d *Deployer) Run(ctx context.Context, req Request) error {
	if err := validateRequest(req); err != nil {
		return err
	}

	if err := d.pullImage(ctx, req); err != nil {
		return err
	}

	prevImage, hadContainer, err := d.currentContainerImage(ctx, req)
	if err != nil {
		return err
	}

	if hadContainer {
		if err := d.removeContainer(ctx, req.Host, req.ContainerName); err != nil {
			return err
		}
	}

	useEnvFile, err := d.shouldUseEnvFile(ctx, req)
	if err != nil {
		return err
	}

	if err := d.runContainer(ctx, req, req.Image, useEnvFile); err != nil {
		return err
	}

	err = health.WaitHTTPReady(ctx, d.runner, health.HTTPCheckRequest{
		Host:     req.Host,
		URL:      req.HealthcheckURL,
		Timeout:  req.HealthcheckTimeout,
		Interval: req.HealthcheckInterval,
	})
	if err == nil {
		return nil
	}

	_ = d.removeContainer(ctx, req.Host, req.ContainerName)

	if strings.TrimSpace(prevImage) == "" {
		return fmt.Errorf("new container failed health checks and was removed: %w", err)
	}

	if rollbackErr := d.runContainer(ctx, req, prevImage, useEnvFile); rollbackErr != nil {
		return fmt.Errorf("new container failed health checks and was removed: %w; rollback failed: %v", err, rollbackErr)
	}

	return fmt.Errorf("new container failed health checks and was removed; rolled back to %q: %w", prevImage, err)
}

func validateRequest(req Request) error {
	if strings.TrimSpace(req.Host) == "" {
		return fmt.Errorf("deploy host is required")
	}
	if strings.TrimSpace(req.ContainerName) == "" {
		return fmt.Errorf("deploy container name is required")
	}
	if strings.TrimSpace(req.Image) == "" {
		return fmt.Errorf("deploy image is required")
	}
	if strings.TrimSpace(req.Port) == "" {
		return fmt.Errorf("deploy port is required")
	}
	if strings.TrimSpace(req.HealthcheckURL) == "" {
		return fmt.Errorf("deploy healthcheck url is required")
	}
	return nil
}

func (d *Deployer) pullImage(ctx context.Context, req Request) error {
	cmd := fmt.Sprintf("docker pull %s", shellQuote(req.Image))
	if _, err := d.runner.Run(ctx, req.Host, cmd); err != nil {
		return fmt.Errorf("pull deploy image %q: %w", req.Image, err)
	}
	return nil
}

func (d *Deployer) currentContainerImage(ctx context.Context, req Request) (string, bool, error) {
	cmd := fmt.Sprintf("docker ps -a --filter name=^/%s$ --format '{{.Image}}'", shellQuote(req.ContainerName))
	result, err := d.runner.Run(ctx, req.Host, cmd)
	if err != nil {
		return "", false, fmt.Errorf("inspect existing container %q: %w", req.ContainerName, err)
	}

	image := strings.TrimSpace(result.Stdout)
	if image == "" {
		return "", false, nil
	}

	firstLine := strings.Split(image, "\n")[0]
	return strings.TrimSpace(firstLine), true, nil
}

func (d *Deployer) shouldUseEnvFile(ctx context.Context, req Request) (bool, error) {
	envFile := strings.TrimSpace(req.EnvFile)
	if envFile == "" {
		return false, nil
	}

	cmd := fmt.Sprintf("if [ -s %s ]; then printf yes; else printf no; fi", shellQuote(envFile))
	result, err := d.runner.Run(ctx, req.Host, cmd)
	if err != nil {
		return false, fmt.Errorf("check env file %q: %w", envFile, err)
	}

	return strings.TrimSpace(result.Stdout) == "yes", nil
}

func (d *Deployer) runContainer(ctx context.Context, req Request, image string, useEnvFile bool) error {
	args := []string{
		"docker run -d",
		"--name", shellQuote(req.ContainerName),
		"-p", shellQuote(req.Port),
	}
	if useEnvFile {
		args = append(args, "--env-file", shellQuote(strings.TrimSpace(req.EnvFile)))
	}
	args = append(args, shellQuote(image))

	cmd := strings.Join(args, " ")
	if _, err := d.runner.Run(ctx, req.Host, cmd); err != nil {
		return fmt.Errorf("run container %q with image %q: %w", req.ContainerName, image, err)
	}

	return nil
}

func (d *Deployer) removeContainer(ctx context.Context, host, containerName string) error {
	cmd := fmt.Sprintf("docker rm -f %s >/dev/null 2>&1 || true", shellQuote(containerName))
	if _, err := d.runner.Run(ctx, host, cmd); err != nil {
		return fmt.Errorf("remove container %q: %w", containerName, err)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(strings.TrimSpace(s), "'", "'\"'\"'") + "'"
}
