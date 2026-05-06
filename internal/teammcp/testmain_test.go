package teammcp

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Package tests set HOME per test. A developer/runtime WUPHF_RUNTIME_HOME
	// must not override that and write broker state into the live office.
	_ = os.Unsetenv("WUPHF_RUNTIME_HOME")
	os.Exit(m.Run())
}
