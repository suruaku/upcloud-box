package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ikox01/upcloud-box/internal/config"
)

func loadConfigOrErr() (*config.Config, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, wrapUserError("load config", err)
	}
	return cfg, nil
}

func logVerbose(format string, args ...any) {
	if !verbose {
		return
	}
	fmt.Printf("[verbose] "+format+"\n", args...)
}

type errKind string

const (
	errUnknown    errKind = "unknown"
	errAuth       errKind = "auth"
	errQuota      errKind = "quota"
	errNetwork    errKind = "network"
	errNotFound   errKind = "not_found"
	errTimeout    errKind = "timeout"
	errSSH        errKind = "ssh"
	errHealth     errKind = "health"
	errValidation errKind = "validation"
)

func classifyError(err error) errKind {
	if err == nil {
		return errUnknown
	}
	msg := strings.ToLower(err.Error())

	if strings.Contains(msg, "invalid config") || strings.Contains(msg, "is required") || strings.Contains(msg, "must be") {
		return errValidation
	}
	if strings.Contains(msg, "upcloud token") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") || strings.Contains(msg, "status=401") || strings.Contains(msg, "status=403") {
		return errAuth
	}
	if strings.Contains(msg, "quota") || strings.Contains(msg, "insufficient") || strings.Contains(msg, "limit") {
		return errQuota
	}
	if strings.Contains(msg, "not found") || strings.Contains(msg, "status=404") || strings.Contains(msg, "status code 404") {
		return errNotFound
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return errTimeout
	}
	if strings.Contains(msg, "ssh") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "no route to host") {
		return errSSH
	}
	if strings.Contains(msg, "health check") || strings.Contains(msg, "unhealthy") {
		return errHealth
	}
	if strings.Contains(msg, "dial tcp") || strings.Contains(msg, "tls") || strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "temporary failure") || strings.Contains(msg, "connection refused") {
		return errNetwork
	}

	return errUnknown
}
func wrapUserError(op string, err error) error {
	if err == nil {
		return nil
	}
	kind := classifyError(err)
	hint := hintForErrKind(kind)
	if verbose {
		return fmt.Errorf("%s failed (%s): %w", op, kind, err)
	}
	if hint != "" {
		return fmt.Errorf("%s failed (%s): %s", op, kind, hint)
	}
	return fmt.Errorf("%s failed (%s)", op, kind)
}

func hintForErrKind(kind errKind) string {
	switch kind {
	case errAuth:
		return "check UPCLOUD_TOKEN and API permissions"
	case errQuota:
		return "check account quota and zone capacity"
	case errNetwork:
		return "check network connectivity and firewall settings"
	case errNotFound:
		return "resource not found; verify state and remote resources"
	case errTimeout:
		return "operation timed out; retry or increase timeout"
	case errSSH:
		return "check SSH user/key, cloud-init user setup, and host reachability"
	case errHealth:
		return "check container logs and healthcheck_url"
	case errValidation:
		return "fix configuration values and retry"
	default:
		return "run with --verbose for detailed error output"
	}
}
