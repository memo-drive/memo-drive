package config

import "testing"

func TestLoadDisablesWebDAVByDefault(t *testing.T) {
	t.Setenv("WEBDAV_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.WebDAV.Enabled {
		t.Fatal("expected WebDAV to be disabled by default")
	}
}

func TestLoadEnablesWebDAVFromEnvironment(t *testing.T) {
	t.Setenv("WEBDAV_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.WebDAV.Enabled {
		t.Fatal("expected WEBDAV_ENABLED=true to enable WebDAV")
	}
}
