package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebDAVDocumentationCoversDeploymentAndClientSmoke(t *testing.T) {
	root := repositoryRoot(t)
	docs := map[string]string{
		"README.md":    readRepoFile(t, root, "README.md"),
		"README-ZH.md": readRepoFile(t, root, "README-ZH.md"),
	}
	required := []string{
		"WEBDAV_ENABLED=true",
		"/dav",
		"OPTIONS, PROPFIND, GET, HEAD, PUT, MKCOL, MOVE, COPY, DELETE",
		"Destination",
		"Depth",
		"Overwrite",
		"If",
		"If-Match",
		"If-None-Match",
		"Authorization",
		"curl -X PROPFIND",
		"curl -T",
		"curl -X MOVE",
		"curl -X DELETE",
		"rclone config create",
		"rclone ls",
		"rclone copy",
		"rclone moveto",
		"rclone deletefile",
		"rclone cat",
	}

	for name, body := range docs {
		t.Run(name, func(t *testing.T) {
			for _, want := range required {
				if !strings.Contains(body, want) {
					t.Fatalf("expected %s to document %q", name, want)
				}
			}
		})
	}
}

func TestWebDAVProductionProxyRoutesDavToBackend(t *testing.T) {
	body := readRepoFile(t, repositoryRoot(t), "deploy", "nginx", "tls.conf")
	for _, want := range []string{
		"location /dav",
		"proxy_pass http://backend:8080",
		"client_max_body_size 0",
		"proxy_request_buffering off",
		"proxy_set_header Destination $http_destination",
		"proxy_set_header Depth $http_depth",
		"proxy_set_header Overwrite $http_overwrite",
		"proxy_set_header If $http_if",
		"proxy_set_header If-Match $http_if_match",
		"proxy_set_header If-None-Match $http_if_none_match",
		"proxy_set_header Authorization $http_authorization",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected production nginx WebDAV proxy config to contain %q", want)
		}
	}
}

func TestWebDAVCurlSmokeScriptCoversFirstVersionWriteFlow(t *testing.T) {
	body := readRepoFile(t, repositoryRoot(t), "deploy", "scripts", "webdav_smoke.sh")
	for _, want := range []string{
		"set -euo pipefail",
		"MEMODRIVE_URL",
		"MEMODRIVE_PASSWORD",
		"curl -X PROPFIND",
		"curl -T",
		"curl -X MOVE",
		"curl -X COPY",
		"curl -X DELETE",
		"/api/files",
		"/trash",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected WebDAV curl smoke script to contain %q", want)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}

func readRepoFile(t *testing.T, root string, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{root}, parts...)...)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}
