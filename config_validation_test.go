package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/Sea-Shell/gogear-api/pkg/utils"
)

// Startup refuses configurations whose admin and client audiences cannot be
// told apart (BR-006 AC-4). The config goes through the real loader.
func TestValidateConfig_FromConfigFile(t *testing.T) {
	for _, tc := range []struct {
		name, auth, wantErr string
	}{
		{"valid", "jwt-audience: client\n  jwt-admin-audience: admin", ""},
		{"admin audience missing", "jwt-audience: client", "jwt-admin-audience must not be empty"},
		{"client audience missing", "jwt-admin-audience: admin", "jwt-audience must not be empty"},
		{"audiences equal", "jwt-audience: same\n  jwt-admin-audience: same", "must differ"},
		{"audiences blank", "jwt-audience: \"  \"\n  jwt-admin-audience: admin", "jwt-audience must not be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(file, []byte("auth:\n  "+tc.auth+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := utils.LoadConfig[models.Config](file)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			err = validateConfig(cfg)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
