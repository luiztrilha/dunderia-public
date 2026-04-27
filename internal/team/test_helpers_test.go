package team

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolateBrokerPersistenceEnv(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "wuphf-test-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 5; i++ {
			err := os.RemoveAll(tmpDir)
			if err == nil || os.IsNotExist(err) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	configDir := filepath.Join(tmpDir, ".wuphf")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	t.Setenv("HOME", tmpDir)
	t.Setenv("WUPHF_RUNTIME_HOME", tmpDir)
	t.Setenv("WUPHF_CONFIG_PATH", filepath.Join(configDir, "config.json"))
	t.Setenv("WUPHF_BROKER_STATE_PATH", filepath.Join(configDir, "team", "broker-state.json"))
	t.Setenv("WUPHF_CLOUD_BACKUP_PROVIDER", "")
	t.Setenv("WUPHF_CLOUD_BACKUP_BUCKET", "")
	t.Setenv("WUPHF_CLOUD_BACKUP_PREFIX", "")
	t.Setenv("WUPHF_CLOUD_BACKUP_BOOTSTRAP_PATH", filepath.Join(configDir, "cloud-backup-bootstrap.json"))

	return tmpDir
}
