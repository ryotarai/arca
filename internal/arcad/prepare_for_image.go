package arcad

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// PrepareForImageHandler returns an HTTP handler that cleans up
// machine-specific state in preparation for image creation.
// arcad itself remains running — it is terminated when the server stops the machine.
func PrepareForImageHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := r.Header.Get("Authorization")
		if token != "Bearer "+cfg.MachineToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := prepareForImage(); err != nil {
			log.Printf("prepare-for-image failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func prepareForImage() error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"stop shelley/ttyd", stopArcaServices},
		{"remove arcad env", func() error { return removeIfExists("/etc/arca/arcad.env") }},
		{"remove arcad state", func() error { return os.RemoveAll("/var/lib/arca") }},
		{"clean cloud-init", func() error { return exec.Command("cloud-init", "clean").Run() }},
		{"remove SSH host keys", removeSSHHostKeys},
		{"truncate machine-id", func() error { return os.WriteFile("/etc/machine-id", []byte(""), 0644) }},
		{"clear user history", clearUserHistory},
	}

	for _, step := range steps {
		log.Printf("prepare-for-image: %s", step.name)
		if err := step.fn(); err != nil {
			log.Printf("prepare-for-image: %s: %v (continuing)", step.name, err)
		}
	}
	return nil
}

func stopArcaServices() error {
	// Stop shelley and ttyd services, but NOT arcad itself
	for _, svc := range []string{"shelley", "ttyd"} {
		cmd := exec.Command("systemctl", "stop", svc)
		if err := cmd.Run(); err != nil {
			log.Printf("stop %s: %v (may not be running)", svc, err)
		}
	}
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeSSHHostKeys() error {
	matches, err := filepath.Glob("/etc/ssh/ssh_host_*")
	if err != nil {
		return err
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			log.Printf("remove %s: %v", m, err)
		}
	}
	return nil
}

func clearUserHistory() error {
	home := "/home/arcauser"
	for _, f := range []string{".bash_history", ".zsh_history"} {
		path := filepath.Join(home, f)
		if err := removeIfExists(path); err != nil {
			log.Printf("remove %s: %v", path, err)
		}
	}
	return nil
}
