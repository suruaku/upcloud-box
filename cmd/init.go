package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ikox01/upcloud-box/internal/config"
	"github.com/ikox01/upcloud-box/internal/state"

	"github.com/spf13/cobra"
)

var initForce bool
var initUser string
var initWriteCloudInit bool
var initCloudInitPath string
var initSSHKeyPaths []string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize local project configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cloudInitPath := ""
		if initWriteCloudInit {
			cloudInitPath = strings.TrimSpace(initCloudInitPath)
			if cloudInitPath == "" {
				return fmt.Errorf("cloud-init path cannot be empty when --write-cloud-init is enabled")
			}
		}

		if err := writeConfig(cfgFile, initForce, cloudInitPath); err != nil {
			return err
		}

		if err := writeState(state.DefaultPath, initForce); err != nil {
			return err
		}

		if initWriteCloudInit {
			keys, err := resolveSSHAuthorizedKeys(initSSHKeyPaths, cfgFile)
			if err != nil {
				return err
			}

			if err := writeCloudInit(cloudInitPath, initUser, keys, initForce); err != nil {
				return err
			}
		}

		fmt.Printf("Created %s\n", cfgFile)
		fmt.Printf("Created %s\n", state.DefaultPath)
		if initWriteCloudInit {
			fmt.Printf("Created %s\n", cloudInitPath)
		}
		fmt.Println("Next: export UPCLOUD_TOKEN, edit your config values, then run upcloud-box provision")
		return nil
	},
}

func init() {
	const defaultCloudInitPath = "./cloud-init.yaml"
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite existing files")
	initCmd.Flags().StringVar(&initUser, "user", "ubuntu", "ssh user to create in cloud-init")
	initCmd.Flags().BoolVar(&initWriteCloudInit, "write-cloud-init", false, "write a cloud-init file and set provision.cloud_init_path")
	initCmd.Flags().StringVar(&initCloudInitPath, "cloud-init-path", defaultCloudInitPath, "path to generated cloud-init file")
	initCmd.Flags().StringSliceVar(&initSSHKeyPaths, "ssh-key", nil, "path to public SSH key file (repeatable)")
	rootCmd.AddCommand(initCmd)
}

func writeConfig(path string, force bool, cloudInitPath string) error {
	if err := config.EnsureParentDir(path); err != nil {
		return err
	}

	if err := ensureWritable(path, force); err != nil {
		return err
	}

	defaultCfg := config.Default()
	defaultCfg.Provision.CloudInitPath = strings.TrimSpace(cloudInitPath)
	data, err := config.MarshalYAML(defaultCfg)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}

	return nil
}

func writeCloudInit(path, user string, sshKeys []string, force bool) error {
	if err := config.EnsureParentDir(path); err != nil {
		return err
	}

	if err := ensureWritable(path, force); err != nil {
		return err
	}

	data := buildCloudInit(user, sshKeys)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write cloud-init %q: %w", path, err)
	}

	return nil
}

func resolveSSHAuthorizedKeys(paths []string, cfgPath string) ([]string, error) {
	if len(paths) > 0 {
		return readSSHAuthorizedKeys(paths)
	}

	inferredPath := inferPublicKeyPathFromConfig(cfgPath)
	if inferredPath != "" {
		keys, err := readSSHAuthorizedKeys([]string{inferredPath})
		if err == nil {
			return keys, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	autoDetectedPublicPath, err := detectDefaultPublicKeyPath()
	if err != nil {
		return nil, err
	}
	if autoDetectedPublicPath != "" {
		return readSSHAuthorizedKeys([]string{autoDetectedPublicPath})
	}

	return nil, fmt.Errorf("no SSH public key found; pass --ssh-key or create ~/.ssh/id_ed25519.pub, ~/.ssh/id_ecdsa.pub, or ~/.ssh/id_rsa.pub")
}

func readSSHAuthorizedKeys(paths []string) ([]string, error) {
	keys := make([]string, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read ssh key file %q: %w", path, err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return nil, fmt.Errorf("ssh key file %q is empty", path)
		}
		keys = append(keys, key)
	}

	return keys, nil
}

func inferPublicKeyPathFromConfig(cfgPath string) string {
	privateKeyPath := config.Default().SSH.PrivateKeyPath

	if cfg, err := config.Load(cfgPath); err == nil {
		if strings.TrimSpace(cfg.SSH.PrivateKeyPath) != "" {
			privateKeyPath = cfg.SSH.PrivateKeyPath
		}
	}

	privateKeyPath = strings.TrimSpace(privateKeyPath)
	if privateKeyPath == "" {
		return ""
	}

	candidate := privateKeyPath
	if !strings.HasSuffix(candidate, ".pub") {
		candidate += ".pub"
	}

	if strings.HasPrefix(candidate, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			candidate = filepath.Join(home, candidate[2:])
		}
	}

	candidate = os.ExpandEnv(candidate)
	if !filepath.IsAbs(candidate) {
		cfgDir := filepath.Dir(cfgPath)
		candidate = filepath.Join(cfgDir, candidate)
	}

	return filepath.Clean(candidate)
}

func detectDefaultPublicKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for ssh key detection: %w", err)
	}

	candidates := []string{"id_ed25519.pub", "id_ecdsa.pub", "id_rsa.pub"}
	for _, name := range candidates {
		candidate := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("check ssh key file %q: %w", candidate, err)
		}
	}

	return "", nil
}

func buildCloudInit(user string, sshKeys []string) []byte {
	trimmedUser := strings.TrimSpace(user)
	if trimmedUser == "" {
		trimmedUser = "ubuntu"
	}

	var b strings.Builder
	b.WriteString("#cloud-config\n")
	b.WriteString("package_update: true\n")
	b.WriteString("package_upgrade: true\n")
	b.WriteString("timezone: UTC\n")
	b.WriteString("ssh_pwauth: false\n")
	b.WriteString("disable_root: true\n")
	b.WriteString("users:\n")
	b.WriteString("  - default\n")
	b.WriteString("  - name: " + trimmedUser + "\n")
	b.WriteString("    gecos: Provisioned by upcloud-box\n")
	b.WriteString("    shell: /bin/bash\n")
	b.WriteString("    lock_passwd: true\n")
	b.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	b.WriteString("    groups: [sudo, docker]\n")
	b.WriteString("    ssh_authorized_keys:\n")
	for _, key := range sshKeys {
		b.WriteString("      - " + key + "\n")
	}
	b.WriteString("packages:\n")
	b.WriteString("  - ca-certificates\n")
	b.WriteString("  - curl\n")
	b.WriteString("  - fail2ban\n")
	b.WriteString("  - ufw\n")
	b.WriteString("  - make\n")
	b.WriteString("  - docker.io\n")
	b.WriteString("write_files:\n")
	b.WriteString("  - path: /etc/ssh/sshd_config.d/99-upcloud-box-hardening.conf\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    content: |\n")
	b.WriteString("      PermitRootLogin no\n")
	b.WriteString("      PasswordAuthentication no\n")
	b.WriteString("      KbdInteractiveAuthentication no\n")
	b.WriteString("      ChallengeResponseAuthentication no\n")
	b.WriteString("      MaxAuthTries 5\n")
	b.WriteString("      AllowTcpForwarding no\n")
	b.WriteString("      X11Forwarding no\n")
	b.WriteString("      AllowAgentForwarding no\n")
	b.WriteString("      ClientAliveInterval 300\n")
	b.WriteString("      ClientAliveCountMax 3\n")
	b.WriteString("      AuthorizedKeysFile .ssh/authorized_keys\n")
	b.WriteString("      AllowUsers " + trimmedUser + "\n")
	b.WriteString("  - path: /etc/fail2ban/jail.local\n")
	b.WriteString("    permissions: '0644'\n")
	b.WriteString("    content: |\n")
	b.WriteString("      [sshd]\n")
	b.WriteString("      enabled = true\n")
	b.WriteString("      port = ssh,22\n")
	b.WriteString("      banaction = iptables-multiport\n")
	b.WriteString("runcmd:\n")
	b.WriteString("  - [sh, -c, \"systemctl reload ssh || systemctl reload sshd || true\"]\n")
	b.WriteString("  - [sh, -c, \"systemctl enable --now fail2ban\"]\n")
	b.WriteString("  - [sh, -c, \"systemctl enable --now docker\"]\n")
	b.WriteString("  - [sh, -c, \"ufw --force default deny incoming\"]\n")
	b.WriteString("  - [sh, -c, \"ufw --force default allow outgoing\"]\n")
	b.WriteString("  - [sh, -c, \"ufw --force allow OpenSSH\"]\n")
	b.WriteString("  - [sh, -c, \"ufw --force allow 80,443/tcp\"]\n")
	b.WriteString("  - [sh, -c, \"ufw --force enable\"]\n")
	b.WriteString("final_message: upcloud-box cloud-init complete\n")

	return []byte(b.String())
}

func writeState(path string, force bool) error {
	if err := ensureWritable(path, force); err != nil {
		return err
	}

	if err := state.Save(path, state.New()); err != nil {
		return err
	}

	return nil
}

func ensureWritable(path string, force bool) error {
	_, err := os.Stat(path)
	if err == nil && !force {
		return fmt.Errorf("file %q already exists (use --force to overwrite)", path)
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("check file %q: %w", path, err)
}
