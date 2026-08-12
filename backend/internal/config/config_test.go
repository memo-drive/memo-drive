package config

import (
	"bytes"
	"log"
	"reflect"
	"strings"
	"testing"
	"time"
)

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

func TestLoadReadsStorageCapacityLimits(t *testing.T) {
	t.Setenv("STORAGE_QUOTA_BYTES", "10485760")
	t.Setenv("STORAGE_RESERVED_BYTES", "2097152")
	t.Setenv("STORAGE_TEMP_LIMIT_BYTES", "4194304")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Storage.QuotaBytes != 10485760 {
		t.Fatalf("quota bytes = %d, want 10485760", cfg.Storage.QuotaBytes)
	}
	if cfg.Storage.ReservedBytes != 2097152 {
		t.Fatalf("reserved bytes = %d, want 2097152", cfg.Storage.ReservedBytes)
	}
	if cfg.Storage.TempLimitBytes != 4194304 {
		t.Fatalf("temp limit bytes = %d, want 4194304", cfg.Storage.TempLimitBytes)
	}
}

func TestLoadReadsDirectoryUploadLimits(t *testing.T) {
	t.Setenv("DIRECTORY_UPLOAD_MAX_ENTRIES", "2500")
	t.Setenv("DIRECTORY_UPLOAD_MAX_DEPTH", "24")
	t.Setenv("DIRECTORY_UPLOAD_MAX_PATH_BYTES", "768")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Storage.DirectoryMaxEntries != 2500 ||
		cfg.Storage.DirectoryMaxDepth != 24 ||
		cfg.Storage.DirectoryMaxPathBytes != 768 {
		t.Fatalf("unexpected directory upload limits %#v", cfg.Storage)
	}
}

func TestLoadRejectsNonPositiveDirectoryUploadLimits(t *testing.T) {
	for _, envName := range []string{
		"DIRECTORY_UPLOAD_MAX_ENTRIES",
		"DIRECTORY_UPLOAD_MAX_DEPTH",
		"DIRECTORY_UPLOAD_MAX_PATH_BYTES",
	} {
		t.Run(envName, func(t *testing.T) {
			t.Setenv(envName, "0")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), envName) {
				t.Fatalf("Load() error = %v, want error naming %s", err, envName)
			}
		})
	}
}

func TestLoadReadsFolderCopyAndZIPLimits(t *testing.T) {
	t.Setenv("FOLDER_COPY_MAX_NODES", "2500")
	t.Setenv("FOLDER_ZIP_MAX_NODES", "3000")
	t.Setenv("FOLDER_ZIP_MAX_UNCOMPRESSED_BYTES", "987654321")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Storage.FolderCopyMaxNodes != 2500 ||
		cfg.Storage.FolderZIPMaxNodes != 3000 ||
		cfg.Storage.FolderZIPMaxUncompressedBytes != 987654321 {
		t.Fatalf("unexpected Folder operation limits %#v", cfg.Storage)
	}
}

func TestLoadReadsFileVersionRetentionSettings(t *testing.T) {
	t.Setenv("FILE_VERSIONING_ENABLED", "false")
	t.Setenv("FILE_VERSION_MAX_COUNT", "12")
	t.Setenv("FILE_VERSION_RETENTION_DAYS", "45")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FileVersion.Enabled || cfg.FileVersion.MaxCount != 12 || cfg.FileVersion.RetentionDays != 45 {
		t.Fatalf("unexpected File Version settings %#v", cfg.FileVersion)
	}
}

func TestLoadRejectsInvalidFileVersionRetentionSettings(t *testing.T) {
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "FILE_VERSION_MAX_COUNT", value: "0"},
		{name: "FILE_VERSION_RETENTION_DAYS", value: "-1"},
	} {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv(item.name, item.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), item.name) {
				t.Fatalf("Load() error = %v, want error naming %s", err, item.name)
			}
		})
	}
}

func TestLoadRejectsNonPositiveFolderCopyAndZIPLimits(t *testing.T) {
	for _, envName := range []string{
		"FOLDER_COPY_MAX_NODES",
		"FOLDER_ZIP_MAX_NODES",
		"FOLDER_ZIP_MAX_UNCOMPRESSED_BYTES",
	} {
		t.Run(envName, func(t *testing.T) {
			t.Setenv(envName, "0")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), envName) {
				t.Fatalf("Load() error = %v, want error naming %s", err, envName)
			}
		})
	}
}

func TestLoadRejectsNegativeStorageCapacityLimits(t *testing.T) {
	for _, envName := range []string{
		"STORAGE_QUOTA_BYTES",
		"STORAGE_RESERVED_BYTES",
		"STORAGE_TEMP_LIMIT_BYTES",
	} {
		t.Run(envName, func(t *testing.T) {
			t.Setenv(envName, "-1")

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), envName) {
				t.Fatalf("Load() error = %v, want error naming %s", err, envName)
			}
		})
	}
}

func TestLoadRejectsDefaultJWTSecretInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", defaultJWTSecret)
	t.Setenv("ADMIN_PASSWORD", "strong-password")
	t.Setenv("EDGE_HTTPS_ENABLED", "true")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Load() error = %v, want production error naming JWT_SECRET", err)
	}
}

func TestLoadRejectsEmptyAdminPasswordInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "production-secret")
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("EDGE_HTTPS_ENABLED", "true")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("Load() error = %v, want production error naming ADMIN_PASSWORD", err)
	}
}

func TestLoadAllowsEmptyAdminPasswordForExplicitTrustedNetworkMode(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "production-secret")
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("TRUSTED_NETWORK_ONLY", "true")
	t.Setenv("EDGE_HTTPS_ENABLED", "true")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v, want trusted-network-only production config to load", err)
	}
}

func TestLoadRejectsProductionWithoutEdgeHTTPS(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "production-secret")
	t.Setenv("ADMIN_PASSWORD", "strong-password")
	t.Setenv("EDGE_HTTPS_ENABLED", "false")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "EDGE_HTTPS_ENABLED") {
		t.Fatalf("Load() error = %v, want production error naming EDGE_HTTPS_ENABLED", err)
	}
}

func TestLoadRejectsUnknownAppEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "staging")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("Load() error = %v, want error naming APP_ENV", err)
	}
}

func TestLoadRejectsWildcardCORSInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "production-secret")
	t.Setenv("ADMIN_PASSWORD", "strong-password")
	t.Setenv("EDGE_HTTPS_ENABLED", "true")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
		t.Fatalf("Load() error = %v, want production error naming CORS_ALLOWED_ORIGINS", err)
	}
}

func TestLoadRejectsCredentialedWildcardCORS(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CORS_ALLOW_CREDENTIALS") {
		t.Fatalf("Load() error = %v, want wildcard credentials incompatibility error", err)
	}
}

func TestLoadAllowsViteOriginsByDefaultInDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for _, want := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
		if !containsString(cfg.CORS.AllowedOrigins, want) {
			t.Fatalf("development CORS origins = %v, want %q", cfg.CORS.AllowedOrigins, want)
		}
	}
}

func TestLoadUsesRateLimitDefaults(t *testing.T) {
	cfg, err := loadWithLookup(func(string) string { return "" })
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.RateLimit.Window != time.Minute ||
		cfg.RateLimit.LoginFailures != 5 ||
		cfg.RateLimit.ReadRequests != 600 ||
		cfg.RateLimit.WriteRequests != 120 ||
		cfg.RateLimit.UploadRequests != 600 ||
		cfg.RateLimit.AIRequests != 30 {
		t.Fatalf("rate limit defaults = %+v", cfg.RateLimit)
	}
}

func TestLoadReadsTrustedProxyCIDRs(t *testing.T) {
	cfg, err := loadWithLookup(func(key string) string {
		if key == "TRUSTED_PROXY_CIDRS" {
			return "10.0.0.0/8, 192.168.1.10/32"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	want := []string{"10.0.0.0/8", "192.168.1.10/32"}
	if !reflect.DeepEqual(cfg.Security.TrustedProxyCIDRs, want) {
		t.Fatalf("trusted proxy CIDRs = %v, want %v", cfg.Security.TrustedProxyCIDRs, want)
	}
}

func TestLoadRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXY_CIDRS") {
		t.Fatalf("Load() error = %v, want error naming TRUSTED_PROXY_CIDRS", err)
	}
}

func TestLoadRejectsNonPositiveRateLimitWindow(t *testing.T) {
	t.Setenv("RATE_LIMIT_WINDOW_SECONDS", "0")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_WINDOW_SECONDS") {
		t.Fatalf("Load() error = %v, want error naming RATE_LIMIT_WINDOW_SECONDS", err)
	}
}

func TestLoadRejectsNonPositiveRateLimitCounts(t *testing.T) {
	for _, envName := range []string{
		"RATE_LIMIT_LOGIN_FAILURES",
		"RATE_LIMIT_READ_REQUESTS",
		"RATE_LIMIT_WRITE_REQUESTS",
		"RATE_LIMIT_UPLOAD_REQUESTS",
		"RATE_LIMIT_AI_REQUESTS",
	} {
		t.Run(envName, func(t *testing.T) {
			t.Setenv(envName, "0")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), envName) {
				t.Fatalf("Load() error = %v, want error naming %s", err, envName)
			}
		})
	}
}

func TestLoadUsesSameOriginOnlyCORSByDefaultInProduction(t *testing.T) {
	cfg, err := loadWithLookup(func(key string) string {
		values := map[string]string{
			"APP_ENV":            "production",
			"JWT_SECRET":         "production-secret",
			"ADMIN_PASSWORD":     "strong-password",
			"EDGE_HTTPS_ENABLED": "true",
		}
		return values[key]
	})
	if err != nil {
		t.Fatalf("load production config: %v", err)
	}
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Fatalf("production default CORS origins = %v, want same-origin-only empty allowlist", cfg.CORS.AllowedOrigins)
	}
}

func TestLoadWarnsAboutWildcardCORSInDevelopment(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousWriter) })
	t.Setenv("APP_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "false")

	if _, err := Load(); err != nil {
		t.Fatalf("load development config: %v", err)
	}
	if !strings.Contains(logs.String(), "insecure_cors_wildcard") {
		t.Fatalf("development warning logs = %q, want insecure_cors_wildcard event", logs.String())
	}
}
