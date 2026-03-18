package arcad

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// ensureAnsible installs ansible via apt if ansible-playbook is not found.
func ensureAnsible(ctx context.Context) error {
	if _, err := exec.LookPath("ansible-playbook"); err == nil {
		return nil
	}
	log.Printf("setup: ansible not found, installing via apt...")
	cmd := exec.CommandContext(ctx, "apt-get", "update")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}
	cmd = exec.CommandContext(ctx, "apt-get", "install", "-y", "--no-install-recommends", "ansible")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apt-get install ansible: %w", err)
	}
	return nil
}

// extractAndRunPlaybook extracts the embedded ansible playbooks to a temp
// directory and runs ansible-playbook. Optional skipTags can be passed to
// skip certain tagged roles.
func extractAndRunPlaybook(ctx context.Context, skipTags ...string) error {
	tmpDir, err := os.MkdirTemp("", "arca-ansible-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := fs.WalkDir(ansibleFS, "ansible", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		destPath := filepath.Join(tmpDir, path)
		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}
		data, err := ansibleFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, 0644)
	}); err != nil {
		return fmt.Errorf("extract ansible files: %w", err)
	}

	playbookPath := filepath.Join(tmpDir, "ansible", "site.yml")
	args := []string{playbookPath, "--connection=local", "-i", "localhost,"}
	if len(skipTags) > 0 {
		tagList := ""
		for i, t := range skipTags {
			if i > 0 {
				tagList += ","
			}
			tagList += t
		}
		args = append(args, "--skip-tags", tagList)
	}
	cmd := exec.CommandContext(ctx, "ansible-playbook", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-playbook: %w", err)
	}
	return nil
}
