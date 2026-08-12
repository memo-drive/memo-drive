package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

func TestWebDAVRouteIsNotRegisteredWhenDisabled(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav", nil))
	if err != nil {
		t.Fatalf("request /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected disabled WebDAV route to return 404, got %d", resp.StatusCode)
	}
}

func TestWebDAVRouteIsRegisteredWhenEnabled(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav", nil))
	if err != nil {
		t.Fatalf("request /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("expected enabled WebDAV route to be registered")
	}
}

func TestWebDAVRouteDoesNotMatchSimilarPrefixes(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/davish", nil))
	if err != nil {
		t.Fatalf("request /davish: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected /davish not to be handled by WebDAV route, got %d", resp.StatusCode)
	}
}

func TestWebDAVRequiresBasicAuthWhenAdminPasswordIsConfigured(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav", nil))
	if err != nil {
		t.Fatalf("request /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected missing WebDAV credentials to return 401, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != `Basic realm="MemoDrive WebDAV"` {
		t.Fatalf("expected WebDAV Basic auth challenge, got %q", got)
	}
}

func TestWebDAVRejectsInvalidBasicAuthCredentials(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/dav", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:wrong")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected invalid WebDAV credentials to return 401, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/dav", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("user:secret")))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("request /dav with wrong user: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected non-admin WebDAV credentials to return 401, got %d", resp.StatusCode)
	}
}

func TestWebDAVAuthFailureLogClassifiesCauseWithoutCredentials(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	tests := []struct {
		name       string
		authHeader string
	}{
		{name: "missing"},
		{name: "unsupported scheme", authHeader: "Bearer secret-token"},
		{name: "malformed basic", authHeader: "Basic not-base64"},
		{name: "invalid credentials", authHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:wrong"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dav", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("GET /dav: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected auth failure to return 401, got %d", resp.StatusCode)
			}
		})
	}

	got := logs.String()
	for _, want := range []string{
		`auth_failure="missing"`,
		`auth_failure="unsupported_scheme"`,
		`auth_failure="malformed_basic"`,
		`auth_failure="invalid_credentials"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected auth failure log to contain %s, got %q", want, got)
		}
	}
	for _, forbidden := range []string{"secret", "wrong", "Authorization"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected auth failure log not to contain %q, got %q", forbidden, got)
		}
	}
}

func TestWebDAVUnauthorizedPutWithBodyClosesConnection(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/large.mov", strings.NewReader("payload"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/large.mov: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected missing auth on PUT to return 401, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Connection"); !strings.EqualFold(got, "close") && !resp.Close {
		t.Fatalf("expected unauthorized PUT with body to close the connection, got Connection %q Close %t", got, resp.Close)
	}
}

func TestWebDAVRateLimitsRepeatedBasicAuthFailures(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	rateLimited := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/dav", nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:wrong")))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request /dav attempt %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("expected rate-limited WebDAV auth failure to include Retry-After")
			}
			break
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected failed auth to return 401 before rate limit, got %d", resp.StatusCode)
		}
	}
	if !rateLimited {
		t.Fatal("expected repeated WebDAV auth failures to be rate limited")
	}

	req := httptest.NewRequest(http.MethodGet, "/dav", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("valid WebDAV auth request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusTooManyRequests {
		t.Fatalf("expected valid WebDAV auth not to be rate limited, got %d", resp.StatusCode)
	}
}

func TestWebDAVAcceptsAdminBasicAuthCredentials(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/dav", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		t.Fatalf("expected valid WebDAV credentials to reach handler, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/dav", nil)
	req.Header.Set("Authorization", "basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("request /dav with lowercase basic scheme: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		t.Fatalf("expected lowercase Basic auth scheme to reach handler, got %d", resp.StatusCode)
	}
}

func TestWebDAVAllowsRequestsWhenAdminPasswordIsNotConfigured(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/note.txt", nil))
	if err != nil {
		t.Fatalf("request /dav/note.txt: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		t.Fatalf("expected WebDAV without admin password to reach handler, got %d", resp.StatusCode)
	}
}

func TestWebDAVIntegrationSuiteCoversFirstVersionProtocol(t *testing.T) {
	app, cleanup := newWebDAVIntegrationTestApp(t)
	defer cleanup()

	missingAuthResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes", nil))
	if err != nil {
		t.Fatalf("unauthenticated PROPFIND /dav/Notes: %v", err)
	}
	defer missingAuthResp.Body.Close()
	if missingAuthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected missing Basic Auth to return 401, got %d", missingAuthResp.StatusCode)
	}

	optionsResp := webDAVIntegrationRequest(t, app, http.MethodOptions, "/dav", nil, nil)
	defer optionsResp.Body.Close()
	if optionsResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected OPTIONS to return 204, got %d", optionsResp.StatusCode)
	}
	assertAllowHeader(t, optionsResp.Header.Get("Allow"), []string{
		"OPTIONS", "PROPFIND", "GET", "HEAD", "PUT", "MKCOL", "MOVE", "COPY", "DELETE",
	})

	unsupportedResp := webDAVIntegrationRequest(t, app, "LOCK", "/dav/Notes/readme.md", nil, nil)
	defer unsupportedResp.Body.Close()
	if unsupportedResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected unsupported LOCK to return 405, got %d", unsupportedResp.StatusCode)
	}

	depthInfinityResp := webDAVIntegrationRequest(t, app, "PROPFIND", "/dav/Notes", nil, map[string]string{"Depth": "infinity"})
	defer depthInfinityResp.Body.Close()
	if depthInfinityResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected Depth: infinity PROPFIND to return 403, got %d", depthInfinityResp.StatusCode)
	}

	propfindBody := strings.NewReader(`<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:displayname/><D:quota-used-bytes/><D:notarealprop/></D:prop></D:propfind>`)
	propfindResp := webDAVIntegrationRequest(t, app, "PROPFIND", "/dav/Notes/readme.md", propfindBody, map[string]string{"Depth": "0"})
	propfindText := readWebDAVIntegrationBody(t, propfindResp)
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", propfindResp.StatusCode, propfindText)
	}
	for _, want := range []string{"<D:multistatus", "<D:displayname>readme.md</D:displayname>", "<D:quota-used-bytes>", "<D:notarealprop", "HTTP/1.1 404 Not Found"} {
		if !strings.Contains(propfindText, want) {
			t.Fatalf("expected PROPFIND XML to contain %q, got %s", want, propfindText)
		}
	}

	dirGetResp := webDAVIntegrationRequest(t, app, http.MethodGet, "/dav/Notes", nil, nil)
	defer dirGetResp.Body.Close()
	if dirGetResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected directory GET to return 405, got %d", dirGetResp.StatusCode)
	}

	dirCopyResp := webDAVIntegrationRequest(t, app, "COPY", "/dav/Notes", nil, map[string]string{"Destination": "http://example.com/dav/NotesCopy"})
	defer dirCopyResp.Body.Close()
	if dirCopyResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected directory COPY to return 201, got %d", dirCopyResp.StatusCode)
	}

	putResp := webDAVIntegrationRequest(t, app, http.MethodPut, "/dav/Notes/integration.md", strings.NewReader("hello integration"), map[string]string{"Content-Type": "text/markdown"})
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected PUT to return 201, got %d", putResp.StatusCode)
	}

	getResp := webDAVIntegrationRequest(t, app, http.MethodGet, "/dav/Notes/integration.md", nil, nil)
	getBody := readWebDAVIntegrationBody(t, getResp)
	if getResp.StatusCode != http.StatusOK || getBody != "hello integration" {
		t.Fatalf("expected GET to return uploaded content, got %d with body %q", getResp.StatusCode, getBody)
	}

	headResp := webDAVIntegrationRequest(t, app, http.MethodHead, "/dav/Notes/integration.md", nil, nil)
	headBody := readWebDAVIntegrationBody(t, headResp)
	if headResp.StatusCode != http.StatusOK || headBody != "" || headResp.Header.Get("ETag") == "" {
		t.Fatalf("expected HEAD to return headers without body, got status %d body %q etag %q", headResp.StatusCode, headBody, headResp.Header.Get("ETag"))
	}

	mkcolResp := webDAVIntegrationRequest(t, app, "MKCOL", "/dav/Notes/Archive", nil, nil)
	defer mkcolResp.Body.Close()
	if mkcolResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected MKCOL to return 201, got %d", mkcolResp.StatusCode)
	}

	moveResp := webDAVIntegrationRequest(t, app, "MOVE", "/dav/Notes/integration.md", nil, map[string]string{"Destination": "http://example.com/dav/Notes/Archive/moved.md"})
	defer moveResp.Body.Close()
	if moveResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected MOVE to return 201, got %d", moveResp.StatusCode)
	}

	copyResp := webDAVIntegrationRequest(t, app, "COPY", "/dav/Notes/Archive/moved.md", nil, map[string]string{"Destination": "http://example.com/dav/Notes/Archive/copy.md"})
	defer copyResp.Body.Close()
	if copyResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected COPY to return 201, got %d", copyResp.StatusCode)
	}

	overwriteResp := webDAVIntegrationRequest(t, app, "COPY", "/dav/Notes/Archive/moved.md", nil, map[string]string{
		"Destination": "http://example.com/dav/Notes/Archive/copy.md",
		"Overwrite":   "F",
	})
	defer overwriteResp.Body.Close()
	if overwriteResp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected COPY Overwrite F conflict to return 412, got %d", overwriteResp.StatusCode)
	}

	conditionalResp := webDAVIntegrationRequest(t, app, http.MethodDelete, "/dav/Notes/Archive/copy.md", nil, map[string]string{"If-Match": `"not-current"`})
	defer conditionalResp.Body.Close()
	if conditionalResp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected conditional DELETE mismatch to return 412, got %d", conditionalResp.StatusCode)
	}

	deleteResp := webDAVIntegrationRequest(t, app, http.MethodDelete, "/dav/Notes/Archive/copy.md", nil, nil)
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected DELETE to return 204, got %d", deleteResp.StatusCode)
	}

	deletedPropfindResp := webDAVIntegrationRequest(t, app, "PROPFIND", "/dav/Notes/Archive/copy.md", nil, nil)
	defer deletedPropfindResp.Body.Close()
	if deletedPropfindResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected deleted file to disappear from WebDAV, got %d", deletedPropfindResp.StatusCode)
	}

	trashResp := webDAVIntegrationRequest(t, app, http.MethodGet, "/trash", nil, nil)
	trashBody := readWebDAVIntegrationBody(t, trashResp)
	if trashResp.StatusCode != http.StatusOK ||
		!strings.Contains(trashBody, `"original_name":"copy.md"`) ||
		!strings.Contains(trashBody, `"original_path":"/Notes/Archive"`) {
		t.Fatalf("expected WebDAV-deleted file to appear in trash, got %d with body %s", trashResp.StatusCode, trashBody)
	}
}

func TestWebDAVOptionsAdvertisesClassOneAndSupportedMethods(t *testing.T) {
	app := fiber.New()
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodOptions, "/dav", nil))
	if err != nil {
		t.Fatalf("OPTIONS /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected OPTIONS /dav to return 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("DAV"); got != "1" {
		t.Fatalf("expected DAV: 1, got %q", got)
	}
	assertAllowHeader(t, resp.Header.Get("Allow"), []string{
		"OPTIONS", "PROPFIND", "GET", "HEAD", "PUT", "MKCOL", "MOVE", "COPY", "DELETE",
	})
	if strings.Contains(resp.Header.Get("DAV"), "2") {
		t.Fatalf("expected WebDAV not to advertise locking class, got DAV header %q", resp.Header.Get("DAV"))
	}
}

func TestWebDAVUnsupportedMethodReturnsMethodNotAllowed(t *testing.T) {
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	for _, method := range []string{"LOCK", "UNLOCK", "PROPPATCH", "REPORT", "SEARCH", http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/dav/note.txt", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("%s /dav/note.txt: %v", method, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("expected unsupported WebDAV method to return 405, got %d", resp.StatusCode)
			}
			assertAllowHeader(t, resp.Header.Get("Allow"), []string{
				"OPTIONS", "PROPFIND", "GET", "HEAD", "PUT", "MKCOL", "MOVE", "COPY", "DELETE",
			})
		})
	}
}

func TestWebDAVUnsupportedMethodLogsStructuredRejectionWithoutCredentials(t *testing.T) {
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest("LOCK", "/dav/note.txt", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	req.Header.Set("User-Agent", "Fileball")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("LOCK /dav/note.txt: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected unsupported WebDAV method to return 405, got %d", resp.StatusCode)
	}

	got := logs.String()
	for _, want := range []string{
		"component=webdav",
		"event=request_begin",
		"method=LOCK",
		`virtual_path="/note.txt"`,
		`user_agent="Fileball"`,
		"event=request_rejected",
		"status=405",
		`reason="method_not_allowed"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected unsupported WebDAV method log to contain %s, got %q", want, got)
		}
	}
	for _, forbidden := range []string{"secret", "Authorization"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected unsupported WebDAV method log not to contain %q, got %q", forbidden, got)
		}
	}
}

func TestWebDAVSupportedCustomMethodReachesHandler(t *testing.T) {
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusMethodNotAllowed {
		t.Fatalf("expected supported custom WebDAV method to reach handler, got %d", resp.StatusCode)
	}
}

func TestWebDAVRejectsDotDotPath(t *testing.T) {
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/..", nil))
	if err != nil {
		t.Fatalf("GET /dav/..: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected dot-dot WebDAV path to return 400, got %d", resp.StatusCode)
	}
}

func TestWebDAVRejectsUnsafeVirtualPaths(t *testing.T) {
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "dot segment", path: "/dav/."},
		{name: "empty segment", path: "/dav/Notes//today.md"},
		{name: "nul byte", path: "/dav/nul%00byte.txt"},
		{name: "encoded slash injection", path: "/dav/a%2Fb.txt"},
		{name: "encoded backslash injection", path: "/dav/a%5Cb.txt"},
		{name: "invalid utf8", path: "/dav/%ff.txt"},
		{name: "trimmed by safety cleaning", path: "/dav/%20note.txt"},
		{name: "rewritten by safety cleaning", path: "/dav/bad%3Aname.txt"},
		{name: "reserved lower trash", path: "/dav/.trash"},
		{name: "reserved mixed trash", path: "/dav/.Trash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, tc.path, nil))
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected unsafe WebDAV path %q to return 400, got %d", tc.path, resp.StatusCode)
			}
		})
	}
}

func TestWebDAVAcceptsRootUnicodeAndDotfileVirtualPaths(t *testing.T) {
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
	})

	for _, path := range []string{
		"/dav",
		"/dav/",
		"/dav/%E6%96%87%E6%A1%A3/%F0%9F%93%84.txt",
		"/dav/.DS_Store",
		"/dav/.obsidian/config.json",
		"/dav/A%26B.txt",
	} {
		t.Run(path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(http.MethodOptions, path, nil))
			if err != nil {
				t.Fatalf("OPTIONS %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("expected valid WebDAV path %q to reach OPTIONS, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestWebDAVMissingVirtualPathReturnsNotFound(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/missing.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/missing.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing WebDAV virtual path to return 404, got %d", resp.StatusCode)
	}
}

func TestWebDAVFindsFileAndFolderByVirtualPath(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	for _, path := range []string{"/dav/Notes", "/dav/Notes/readme.md"} {
		t.Run(path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("PROPFIND", path, nil))
			if err != nil {
				t.Fatalf("PROPFIND %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusMultiStatus {
				t.Fatalf("expected existing WebDAV path %q to return PROPFIND multistatus, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestWebDAVFindsNestedVirtualPathCaseInsensitively(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/notes/README.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/notes/README.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected case-insensitive WebDAV virtual path to resolve, got %d", resp.StatusCode)
	}
}

func TestWebDAVPropfindEmptyBodyReturnsMultistatusForFolder(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/xml") {
		t.Fatalf("expected XML Content-Type, got %q", got)
	}
	for _, want := range []string{
		`<D:multistatus xmlns:D="DAV:">`,
		`<D:href>/dav/Notes/</D:href>`,
		`<D:displayname>Notes</D:displayname>`,
		`<D:resourcetype><D:collection></D:collection></D:resourcetype>`,
		`<D:status>HTTP/1.1 200 OK</D:status>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected PROPFIND XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindDepthZeroReturnsCurrentFileProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil)
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	if got := strings.Count(text, "<D:response>"); got != 1 {
		t.Fatalf("expected Depth: 0 to return one response, got %d in %s", got, text)
	}
	for _, want := range []string{
		`<D:href>/dav/Notes/readme.md</D:href>`,
		`<D:creationdate>`,
		`<D:displayname>readme.md</D:displayname>`,
		`<D:getcontentlanguage></D:getcontentlanguage>`,
		`<D:getcontentlength>6</D:getcontentlength>`,
		`<D:getcontenttype>text/markdown</D:getcontenttype>`,
		`<D:getetag>`,
		`<D:getlastmodified>`,
		`<D:lockdiscovery></D:lockdiscovery>`,
		`<D:resourcetype></D:resourcetype>`,
		`<D:supportedlock></D:supportedlock>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected PROPFIND XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindDepthOneReturnsFolderAndDirectChildren(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes", nil)
	req.Header.Set("Depth", "1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	if got := strings.Count(text, "<D:response>"); got != 2 {
		t.Fatalf("expected Depth: 1 to return folder and direct child, got %d in %s", got, text)
	}
	for _, want := range []string{
		`<D:href>/dav/Notes/</D:href>`,
		`<D:href>/dav/Notes/readme.md</D:href>`,
		`<D:displayname>readme.md</D:displayname>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected PROPFIND XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindDepthOneDefaultsToNewestUploadsFirst(t *testing.T) {
	app, db, storageRoot, cleanup := newWebDAVLookupTestAppWithMaxFileSize(t, 1024*1024)
	defer cleanup()
	createHandlerTestFile(t, db, storageRoot, &model.File{
		ID:          "old-upload",
		Name:        "a-old.txt",
		Path:        "/Notes",
		StoragePath: "Notes/a-old.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "old")
	time.Sleep(time.Millisecond)
	createHandlerTestFile(t, db, storageRoot, &model.File{
		ID:          "new-upload",
		Name:        "z-new.txt",
		Path:        "/Notes",
		StoragePath: "Notes/z-new.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "new")

	req := httptest.NewRequest("PROPFIND", "/dav/Notes", nil)
	req.Header.Set("Depth", "1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes: %v", err)
	}
	text := readWebDAVIntegrationBody(t, resp)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	newIndex := strings.Index(text, "<D:href>/dav/Notes/z-new.txt</D:href>")
	oldIndex := strings.Index(text, "<D:href>/dav/Notes/a-old.txt</D:href>")
	if newIndex < 0 || oldIndex < 0 {
		t.Fatalf("expected PROPFIND body to contain both uploaded files, got %s", text)
	}
	if newIndex > oldIndex {
		t.Fatalf("expected newest upload href before older upload href, got %s", text)
	}
}

func TestWebDAVPropfindDepthInfinityReturnsForbidden(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes", nil)
	req.Header.Set("Depth", "infinity")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected Depth: infinity to return 403, got %d", resp.StatusCode)
	}
}

func TestWebDAVPropfindPropnameReturnsOnlyPropertyNames(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", strings.NewReader(`<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:propname/></D:propfind>`))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND propname: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	if strings.Contains(text, "<D:displayname>readme.md</D:displayname>") {
		t.Fatalf("expected propname response to omit property values, got %s", text)
	}
	for _, want := range []string{
		`<D:creationdate></D:creationdate>`,
		`<D:displayname></D:displayname>`,
		`<D:getcontentlength></D:getcontentlength>`,
		`<D:resourcetype></D:resourcetype>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected propname XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindAllpropRequestReturnsSupportedProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", strings.NewReader(`<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND allprop: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:displayname>readme.md</D:displayname>`,
		`<D:getcontentlength>6</D:getcontentlength>`,
		`<D:getcontenttype>text/markdown</D:getcontenttype>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected allprop XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindAllpropIncludesQuotaProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes", nil))
	if err != nil {
		t.Fatalf("PROPFIND allprop quota: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	if !strings.Contains(text, `<D:quota-used-bytes>6</D:quota-used-bytes>`) {
		t.Fatalf("expected allprop to include quota-used-bytes, got %s", text)
	}
	availableText := webDAVTestPropertyValue(t, text, "quota-available-bytes")
	available, err := strconv.ParseInt(availableText, 10, 64)
	if err != nil {
		t.Fatalf("expected numeric quota-available-bytes, got %q in %s", availableText, text)
	}
	if available <= 0 {
		t.Fatalf("expected positive quota-available-bytes, got %d in %s", available, text)
	}
}

func TestWebDAVPropfindAllpropIncludeReturnsSupportedProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	body := `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
  <D:allprop/>
  <D:include>
    <D:quota-used-bytes/>
    <D:quota-available-bytes/>
  </D:include>
</D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND allprop include /dav/: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected allprop include PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:displayname></D:displayname>`,
		`<D:quota-used-bytes>`,
		`<D:quota-available-bytes>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected allprop include XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindIncludeBeforeAllpropReturnsSupportedProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	body := `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
  <D:include>
    <D:quota-used-bytes/>
    <D:quota-available-bytes/>
  </D:include>
  <D:allprop/>
</D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND include allprop /dav/: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected include allprop PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:displayname></D:displayname>`,
		`<D:quota-used-bytes>`,
		`<D:quota-available-bytes>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected include allprop XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindAllpropPropFallbackReturnsSupportedProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	body := `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
  <D:allprop/>
  <D:prop>
    <D:quota-used-bytes/>
    <D:quota-available-bytes/>
  </D:prop>
</D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND allprop prop fallback /dav/: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected allprop prop fallback PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:displayname></D:displayname>`,
		`<D:quota-used-bytes>`,
		`<D:quota-available-bytes>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected allprop prop fallback XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindPropBeforeAllpropReturnsSupportedProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	body := `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:displayname/>
    <D:getcontentlength/>
    <D:getlastmodified/>
  </D:prop>
  <D:allprop/>
</D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND prop allprop /dav/: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected prop allprop PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:displayname></D:displayname>`,
		`<D:getcontentlength>0</D:getcontentlength>`,
		`<D:resourcetype><D:collection></D:collection></D:resourcetype>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected prop allprop XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVPropfindLogsStructuredSuccessWithoutCredentials(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes", nil)
	req.Header.Set("Depth", "1")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d", resp.StatusCode)
	}

	got := logs.String()
	for _, want := range []string{
		"component=webdav",
		"event=propfind_complete",
		"method=PROPFIND",
		`virtual_path="/Notes"`,
		`depth="1"`,
		"mode=allprop",
		"props=0",
		"resources=2",
		"status=207",
		"duration_ms=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected PROPFIND success log to contain %s, got %q", want, got)
		}
	}
	for _, forbidden := range []string{"secret", "Authorization"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected PROPFIND success log not to contain %q, got %q", forbidden, got)
		}
	}
}

func TestWebDAVPropfindParseFailureLogsStructuredRejectionWithoutBody(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes", strings.NewReader(`<D:propfind xmlns:D="DAV:"><D:prop>`))
	req.Header.Set("Depth", "0")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND malformed XML: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected malformed PROPFIND XML to return 400, got %d", resp.StatusCode)
	}

	got := logs.String()
	for _, want := range []string{
		"component=webdav",
		"event=propfind_rejected",
		"method=PROPFIND",
		`virtual_path="/Notes"`,
		`depth="0"`,
		"status=400",
		`reason="parse_error"`,
		"body_bytes=35",
		"err=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected PROPFIND rejection log to contain %s, got %q", want, got)
		}
	}
	for _, forbidden := range []string{"secret", "Authorization", "<D:propfind"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected PROPFIND rejection log not to contain %q, got %q", forbidden, got)
		}
	}
}

func TestWebDAVPropfindExplicitPropGroupsUnknownProperties(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:displayname/><D:notarealprop/></D:prop></D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND explicit prop: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:displayname>readme.md</D:displayname>`,
		`<D:status>HTTP/1.1 200 OK</D:status>`,
		`<D:notarealprop></D:notarealprop>`,
		`<D:status>HTTP/1.1 404 Not Found</D:status>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected explicit prop XML to contain %q, got %s", want, text)
		}
	}
	if strings.Contains(text, "<D:getcontentlength>") {
		t.Fatalf("expected explicit prop response not to include unrequested properties, got %s", text)
	}
}

func TestWebDAVPropfindExplicitQuotaPropertiesReturnStorageUsage(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:quota-used-bytes/><D:quota-available-bytes/></D:prop></D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/Notes", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND explicit quota prop: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	if !strings.Contains(text, `<D:quota-used-bytes>6</D:quota-used-bytes>`) {
		t.Fatalf("expected quota-used-bytes to use active file size, got %s", text)
	}
	availableText := webDAVTestPropertyValue(t, text, "quota-available-bytes")
	available, err := strconv.ParseInt(availableText, 10, 64)
	if err != nil {
		t.Fatalf("expected numeric quota-available-bytes, got %q in %s", availableText, text)
	}
	if available <= 0 {
		t.Fatalf("expected positive quota-available-bytes, got %d in %s", available, text)
	}
	if strings.Contains(text, `HTTP/1.1 404 Not Found`) {
		t.Fatalf("expected quota properties to be supported, got %s", text)
	}
}

func TestWebDAVQuotaAvailableUsesCapacityAdmissionResult(t *testing.T) {
	properties := webDAVQuotaProperties(&service.StorageUsage{
		UsedBytes:            80,
		TotalBytes:           100,
		UploadAvailableBytes: 7,
	})
	for _, property := range properties {
		if property.Name == "quota-available-bytes" {
			if property.Value != "7" {
				t.Fatalf(
					"quota-available-bytes = %q, want admission result 7",
					property.Value,
				)
			}
			return
		}
	}
	t.Fatal("quota-available-bytes property missing")
}

func TestWebDAVPropfindQuotaFailureDoesNotHideOtherRequestedProperties(t *testing.T) {
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  1024 * 1024,
			ChunkSize:    5,
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")

	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, cfg, service.NewWebDAVService(cfg, db))
	if err := os.RemoveAll(storageRoot); err != nil {
		t.Fatalf("remove storage root: %v", err)
	}

	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:displayname/><D:quota-used-bytes/><D:quota-available-bytes/></D:prop></D:propfind>`
	req := httptest.NewRequest("PROPFIND", "/dav/Notes", strings.NewReader(body))
	req.Header.Set("Depth", "0")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND quota failure: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(responseBody)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	okPropstat := webDAVTestPropstatWithStatus(t, text, "HTTP/1.1 200 OK")
	if !strings.Contains(okPropstat, `<D:displayname>Notes</D:displayname>`) {
		t.Fatalf("expected non-quota property to remain in 200 propstat, got %s", text)
	}
	failedPropstat := webDAVTestPropstatWithStatus(t, text, "HTTP/1.1 500 Internal Server Error")
	for _, want := range []string{
		`<D:quota-used-bytes></D:quota-used-bytes>`,
		`<D:quota-available-bytes></D:quota-available-bytes>`,
	} {
		if !strings.Contains(failedPropstat, want) {
			t.Fatalf("expected failed quota propstat to contain %q, got %s", want, text)
		}
	}
	if strings.Contains(text, `HTTP/1.1 404 Not Found`) {
		t.Fatalf("expected quota failure not to be treated as unknown property, got %s", text)
	}
}

func TestWebDAVPropfindMalformedXMLReturnsBadRequest(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("PROPFIND", "/dav/Notes", strings.NewReader(`<D:propfind xmlns:D="DAV:"><D:prop>`))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND malformed XML: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected malformed PROPFIND XML to return 400, got %d", resp.StatusCode)
	}
}

func TestWebDAVPropfindEscapesDisplayNameAndHref(t *testing.T) {
	app, db, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()
	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "ampersand",
		Name:        "A&B.txt",
		Path:        "/Notes",
		StoragePath: "Notes/A&B.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create ampersand file: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/A%26B.txt", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/A%%26B.txt: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	text := string(body)
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", resp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:href>/dav/Notes/A&amp;B.txt</D:href>`,
		`<D:displayname>A&amp;B.txt</D:displayname>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected escaped PROPFIND XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVGetFileReturnsContentAndDownloadHeaders(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected WebDAV file GET to return 200, got %d with body %s", resp.StatusCode, string(body))
	}
	if string(body) != "readme" {
		t.Fatalf("expected WebDAV GET body %q, got %q", "readme", body)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/markdown" {
		t.Fatalf("expected text/markdown content type, got %q", got)
	}
	if got := resp.Header.Get("Content-Length"); got != "6" {
		t.Fatalf("expected content length 6, got %q", got)
	}
	if got := resp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("expected Accept-Ranges bytes, got %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("expected inline content disposition, got %q", got)
	}
	if got := resp.Header.Get("ETag"); got == "" {
		t.Fatal("expected ETag header")
	}
	if got := resp.Header.Get("Last-Modified"); got == "" {
		t.Fatal("expected Last-Modified header")
	}
}

func TestWebDAVHeadFileReturnsHeadersWithoutBody(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodHead, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("HEAD /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read HEAD body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected WebDAV file HEAD to return 200, got %d", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if got := resp.Header.Get("Content-Length"); got != "6" {
		t.Fatalf("expected content length 6, got %q", got)
	}
	if got := resp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("expected Accept-Ranges bytes, got %q", got)
	}
	if got := resp.Header.Get("ETag"); got == "" {
		t.Fatal("expected ETag header")
	}
	if got := resp.Header.Get("Last-Modified"); got == "" {
		t.Fatal("expected Last-Modified header")
	}
}

func TestWebDAVGetFileRangeReturnsPartialContent(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil)
	req.Header.Set("Range", "bytes=1-3")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET range /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read range body: %v", err)
	}
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected WebDAV range GET to return 206, got %d with body %s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 1-3/6" {
		t.Fatalf("expected Content-Range bytes 1-3/6, got %q", got)
	}
	if got := resp.Header.Get("Content-Length"); got != "3" {
		t.Fatalf("expected content length 3, got %q", got)
	}
	if string(body) != "ead" {
		t.Fatalf("expected range body %q, got %q", "ead", body)
	}
}

func TestWebDAVGetFolderReturnsMethodNotAllowed(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected WebDAV folder GET to return 405, got %d", resp.StatusCode)
	}
	assertAllowHeader(t, resp.Header.Get("Allow"), []string{
		"OPTIONS", "PROPFIND", "GET", "HEAD", "PUT", "MKCOL", "MOVE", "COPY", "DELETE",
	})
}

func TestWebDAVGetMissingFileReturnsNotFound(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/missing.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/missing.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing WebDAV file GET to return 404, got %d", resp.StatusCode)
	}
}

func TestWebDAVETagIsConsistentAcrossGetHeadAndPropfind(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/readme.md: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected GET to return 200, got %d", getResp.StatusCode)
	}
	getETag := getResp.Header.Get("ETag")
	if getETag == "" {
		t.Fatal("expected GET ETag")
	}

	headResp, err := app.Test(httptest.NewRequest(http.MethodHead, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("HEAD /dav/Notes/readme.md: %v", err)
	}
	defer headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		t.Fatalf("expected HEAD to return 200, got %d", headResp.StatusCode)
	}
	if got := headResp.Header.Get("ETag"); got != getETag {
		t.Fatalf("expected HEAD ETag %q to match GET ETag %q", got, getETag)
	}

	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:getetag/></D:prop></D:propfind>`
	propfindReq := httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", strings.NewReader(body))
	propfindReq.Header.Set("Depth", "0")
	propfindResp, err := app.Test(propfindReq)
	if err != nil {
		t.Fatalf("PROPFIND getetag /dav/Notes/readme.md: %v", err)
	}
	defer propfindResp.Body.Close()
	propfindBody, err := io.ReadAll(propfindResp.Body)
	if err != nil {
		t.Fatalf("read PROPFIND body: %v", err)
	}
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d with body %s", propfindResp.StatusCode, string(propfindBody))
	}
	if got := html.UnescapeString(webDAVTestPropertyValue(t, string(propfindBody), "getetag")); got != getETag {
		t.Fatalf("expected PROPFIND getetag %q to match GET ETag %q", got, getETag)
	}
}

func TestWebDAVETagChangesWhenFileStateChanges(t *testing.T) {
	app, db, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	firstResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET first ETag: %v", err)
	}
	defer firstResp.Body.Close()
	firstETag := firstResp.Header.Get("ETag")
	if firstETag == "" {
		t.Fatal("expected first ETag")
	}

	time.Sleep(time.Millisecond)
	if err := db.UpdateFileChunkCount(context.Background(), "readme", 2); err != nil {
		t.Fatalf("update file state: %v", err)
	}

	secondResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET second ETag: %v", err)
	}
	defer secondResp.Body.Close()
	secondETag := secondResp.Header.Get("ETag")
	if secondETag == "" {
		t.Fatal("expected second ETag")
	}
	if secondETag == firstETag {
		t.Fatalf("expected ETag to change after file state changes, still got %q", secondETag)
	}
}

func TestWebDAVWriteIfMatchMismatchReturnsPreconditionFailed(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/dav/Notes/readme.md", nil)
	req.Header.Set("If-Match", `"not-current"`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/readme.md with If-Match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected If-Match mismatch to return 412, got %d", resp.StatusCode)
	}
}

func TestWebDAVPutIfNoneMatchStarBlocksExistingTarget(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("new content"))
	req.Header.Set("If-None-Match", "*")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/readme.md with If-None-Match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected If-None-Match * on existing target to return 412, got %d", resp.StatusCode)
	}
}

func TestWebDAVWriteSimpleIfETagMismatchReturnsPreconditionFailed(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/dav/Notes/readme.md", nil)
	req.Header.Set("If", `(["not-current"])`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/readme.md with If: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected simple If ETag mismatch to return 412, got %d", resp.StatusCode)
	}
}

func TestWebDAVWriteComplexIfHeaderIsRejectedAndLogged(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodDelete, "/dav/Notes/readme.md", nil)
	req.Header.Set("If", `(<opaquelocktoken:123>)`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/readme.md with lock token If: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected complex If header to return 412, got %d", resp.StatusCode)
	}
	if got := logs.String(); !strings.Contains(got, "event=if_header_rejected") {
		t.Fatalf("expected complex If rejection to be logged, got %q", got)
	}
}

func TestWebDAVPutNewFileCreatesFile(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/new.md", strings.NewReader("# New\n"))
	req.Header.Set("Content-Type", "text/markdown")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/new.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected new WebDAV PUT to return 201, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/new.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/new.md: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read new file body: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected created WebDAV file to be readable, got %d with body %s", getResp.StatusCode, string(body))
	}
	if string(body) != "# New\n" {
		t.Fatalf("expected created file content %q, got %q", "# New\n", body)
	}
	if got := getResp.Header.Get("Content-Type"); got != "text/markdown" {
		t.Fatalf("expected created file content type text/markdown, got %q", got)
	}
}

func TestWebDAVPutRootFileWithoutSlashAfterMountCreatesFileAndLogsCompat(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPut, "/davroot.md", strings.NewReader("root"))
	req.Header.Set("Content-Type", "text/markdown")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /davroot.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected mount-compat root PUT to return 201, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/root.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/root.md: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read mount-compat root file body: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "root" {
		t.Fatalf("expected mount-compat root file to be readable, got %d with body %q", getResp.StatusCode, body)
	}

	got := logs.String()
	for _, want := range []string{
		"event=request_begin",
		"method=PUT",
		`path="/davroot.md"`,
		`path_compat="missing_slash_after_mount"`,
		`virtual_path="/root.md"`,
		"event=write_complete",
		"status=201",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected mount-compat root PUT log to contain %s, got %q", want, got)
		}
	}
}

func TestWebDAVPutRootFileWithoutMountPrefixCreatesFileAndLogsCompat(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPut, "/root-direct.md", strings.NewReader("root direct"))
	req.Header.Set("Content-Type", "text/markdown")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /root-direct.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected root-compat PUT to return 201, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/root-direct.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/root-direct.md: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read root-compat file body: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "root direct" {
		t.Fatalf("expected root-compat file to be readable, got %d with body %q", getResp.StatusCode, body)
	}

	got := logs.String()
	for _, want := range []string{
		"event=request_begin",
		"method=PUT",
		`path="/root-direct.md"`,
		`path_compat="missing_mount_prefix"`,
		`virtual_path="/root-direct.md"`,
		"event=write_complete",
		"status=201",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected root-compat PUT log to contain %s, got %q", want, got)
		}
	}
}

func TestWebDAVPutLogsStructuredWriteResultWithoutCredentials(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/logged.md", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/logged.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected PUT to return 201, got %d", resp.StatusCode)
	}

	got := logs.String()
	for _, want := range []string{
		"component=webdav",
		"event=write_complete",
		"method=PUT",
		`virtual_path="/Notes/logged.md"`,
		`destination_path=""`,
		"file_id=",
		"bytes=5",
		"status=201",
		"duration_ms=",
		`err=""`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected WebDAV PUT write log to contain %s, got %q", want, got)
		}
	}
	for _, forbidden := range []string{"secret", "Authorization"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected WebDAV write log not to contain %q, got %q", forbidden, got)
		}
	}
}

func TestWebDAVWriteMethodsLogStructuredResults(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		path            string
		destination     string
		wantStatus      int
		wantVirtual     string
		wantDestination string
		wantBytes       string
	}{
		{
			name:        "mkcol",
			method:      "MKCOL",
			path:        "/dav/Notes/LoggedFolder",
			wantStatus:  http.StatusCreated,
			wantVirtual: "/Notes/LoggedFolder",
			wantBytes:   "bytes=0",
		},
		{
			name:            "move",
			method:          "MOVE",
			path:            "/dav/Notes/readme.md",
			destination:     "http://example.com/dav/Notes/moved-log.md",
			wantStatus:      http.StatusCreated,
			wantVirtual:     "/Notes/readme.md",
			wantDestination: "/Notes/moved-log.md",
			wantBytes:       "bytes=0",
		},
		{
			name:            "copy",
			method:          "COPY",
			path:            "/dav/Notes/readme.md",
			destination:     "http://example.com/dav/Notes/copied-log.md",
			wantStatus:      http.StatusCreated,
			wantVirtual:     "/Notes/readme.md",
			wantDestination: "/Notes/copied-log.md",
			wantBytes:       "bytes=6",
		},
		{
			name:        "delete",
			method:      http.MethodDelete,
			path:        "/dav/Notes/readme.md",
			wantStatus:  http.StatusNoContent,
			wantVirtual: "/Notes/readme.md",
			wantBytes:   "bytes=0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, _, cleanup := newWebDAVLookupTestApp(t)
			defer cleanup()

			var logs bytes.Buffer
			previousOutput := log.Writer()
			previousFlags := log.Flags()
			log.SetOutput(&logs)
			log.SetFlags(0)
			defer func() {
				log.SetOutput(previousOutput)
				log.SetFlags(previousFlags)
			}()

			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.destination != "" {
				req.Header.Set("Destination", tc.destination)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("expected %s to return %d, got %d", tc.method, tc.wantStatus, resp.StatusCode)
			}

			got := logs.String()
			for _, want := range []string{
				"event=write_complete",
				"method=" + tc.method,
				`virtual_path="` + tc.wantVirtual + `"`,
				`destination_path="` + tc.wantDestination + `"`,
				"file_id=",
				tc.wantBytes,
				"status=" + strconv.Itoa(tc.wantStatus),
				"duration_ms=",
				`err=""`,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("expected WebDAV write log to contain %s, got %q", want, got)
				}
			}
		})
	}
}

func TestWebDAVWriteFailureLogsStatusAndError(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE /dav/Notes/readme.md without Destination: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid MOVE to return 400, got %d", resp.StatusCode)
	}

	got := logs.String()
	for _, want := range []string{
		"event=write_complete",
		"method=MOVE",
		`virtual_path="/Notes/readme.md"`,
		`destination_path=""`,
		"file_id=readme",
		"bytes=0",
		"status=400",
		`err="invalid destination: missing"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected failed WebDAV write log to contain %s, got %q", want, got)
		}
	}
}

func TestWebDAVPutExistingFileOverwritesContentAndKeepsFileID(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	beforeResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/readme.md before overwrite: %v", err)
	}
	defer beforeResp.Body.Close()
	beforeETag := beforeResp.Header.Get("ETag")
	if beforeETag == "" {
		t.Fatal("expected ETag before overwrite")
	}

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("rewritten"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT overwrite /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected WebDAV overwrite PUT to return 204, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/readme.md after overwrite: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected overwritten file to be readable, got %d with body %s", getResp.StatusCode, string(body))
	}
	if string(body) != "rewritten" {
		t.Fatalf("expected overwritten body %q, got %q", "rewritten", body)
	}
	if got := getResp.Header.Get("Content-Length"); got != "9" {
		t.Fatalf("expected overwritten content length 9, got %q", got)
	}
	if got := getResp.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("expected overwritten content type text/plain, got %q", got)
	}
	afterETag := getResp.Header.Get("ETag")
	if afterETag == "" || afterETag == beforeETag {
		t.Fatalf("expected overwrite to change ETag, before=%q after=%q", beforeETag, afterETag)
	}

	fileResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/readme", nil))
	if err != nil {
		t.Fatalf("GET /files/readme after overwrite: %v", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("expected original File ID to remain readable, got %d", fileResp.StatusCode)
	}
	var file model.File
	if err := json.NewDecoder(fileResp.Body).Decode(&file); err != nil {
		t.Fatalf("decode overwritten file: %v", err)
	}
	if file.ID != "readme" || file.Size != 9 || file.MimeType != "text/plain" || file.Status != model.FileStatusUploaded {
		t.Fatalf("expected original File ID with updated metadata, got %#v", file)
	}
}

func TestWebDAVPutRecordsFileMutation(t *testing.T) {
	app, db, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("journaled"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT overwrite /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected overwrite to return 204, got %d", resp.StatusCode)
	}
	ids, err := db.ListFileMutationIDs(context.Background())
	if err != nil {
		t.Fatalf("list File Mutation journal IDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected WebDAV PUT to record one File Mutation, got %d", len(ids))
	}
}

func TestWebDAVPutExistingFileClearsOldIndexData(t *testing.T) {
	app, db, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()
	thumb := "thumbs/readme.png"
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{
		FileID:        "readme",
		MetaJSON:      `{"old":"metadata"}`,
		ThumbnailPath: &thumb,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	if err := db.UpsertChunks(context.Background(), []store.ChunkRow{
		{ID: "readme#parent-0", FileID: "readme", FileName: "readme.md", Heading: "Old", ChunkIndex: 0, Text: "old parent token", IsParent: true},
		{ID: "readme#0", FileID: "readme", FileName: "readme.md", Heading: "Old", ChunkIndex: 0, Text: "old-token", ParentChunkID: "readme#parent-0"},
	}); err != nil {
		t.Fatalf("seed chunks: %v", err)
	}
	if err := db.UpdateFileChunkCount(context.Background(), "readme", 1); err != nil {
		t.Fatalf("seed chunk count: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("fresh"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT overwrite /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected overwrite to return 204, got %d", resp.StatusCode)
	}

	metaResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/readme/metadata", nil))
	if err != nil {
		t.Fatalf("GET /files/readme/metadata: %v", err)
	}
	defer metaResp.Body.Close()
	if metaResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected overwrite to clear old metadata, got %d", metaResp.StatusCode)
	}
	chunks, err := db.SearchChunksBM25(context.Background(), "old-token", nil, 10)
	if err != nil {
		t.Fatalf("search old chunks: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected overwrite to clear old chunks, got %#v", chunks)
	}
	fileResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/readme", nil))
	if err != nil {
		t.Fatalf("GET /files/readme: %v", err)
	}
	defer fileResp.Body.Close()
	var file model.File
	if err := json.NewDecoder(fileResp.Body).Decode(&file); err != nil {
		t.Fatalf("decode file after index cleanup: %v", err)
	}
	if file.ChunkCount != 0 {
		t.Fatalf("expected overwrite to reset chunk count, got %d", file.ChunkCount)
	}
}

func TestWebDAVPutExistingFileDeletesOldVectorChunks(t *testing.T) {
	vector := &webDAVRecordingVectorStore{}
	app, db, cleanup := newWebDAVLookupTestAppWithVectorStore(t, vector)
	defer cleanup()
	if err := db.UpdateFileChunkCount(context.Background(), "readme", 2); err != nil {
		t.Fatalf("seed chunk count: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("fresh vectors"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT overwrite /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected overwrite to return 204, got %d", resp.StatusCode)
	}
	want := []string{"readme#0", "readme#1"}
	if strings.Join(vector.deletedIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("expected old vector chunks %v to be deleted, got %v", want, vector.deletedIDs)
	}
}

func TestWebDAVPutExistingFileRecordsWebDAVVersionSource(t *testing.T) {
	app, _, _, cleanup := newWebDAVLookupTestAppWithConfig(t, 1024, nil, func(cfg *config.Config) {
		cfg.FileVersion.Enabled = true
	})
	defer cleanup()

	putReq := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("rewritten"))
	putReq.Header.Set("Content-Type", "text/markdown")
	putResp, err := app.Test(putReq)
	if err != nil {
		t.Fatalf("PUT existing WebDAV File: %v", err)
	}
	_ = putResp.Body.Close()
	if putResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT existing WebDAV File status = %d, want 204", putResp.StatusCode)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/files/readme/versions", nil)
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list WebDAV File Versions: %v", err)
	}
	defer listResp.Body.Close()
	var body struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode WebDAV File Versions: %v", err)
	}
	if len(body.Versions) != 1 || body.Versions[0].Source != "webdav_put" {
		t.Fatalf("unexpected WebDAV File Versions %#v", body.Versions)
	}
}

func TestWebDAVPutExistingFileKeepsOverwriteWhenPipelineEnqueueFails(t *testing.T) {
	app, cleanup := newWebDAVLookupTestAppWithStoppedPipeline(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("overwrite still saves"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT overwrite /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected overwrite to return 204 even when pipeline enqueue fails, got %d", resp.StatusCode)
	}
	if got := logs.String(); !strings.Contains(got, "event=pipeline_enqueue_failed") {
		t.Fatalf("expected pipeline enqueue failure to be logged, got %q", got)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/readme.md: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if string(body) != "overwrite still saves" {
		t.Fatalf("expected overwrite to persist despite pipeline failure, got %q", body)
	}
}

func TestWebDAVPutExistingFileIfMatchMismatchDoesNotReplaceContent(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/readme.md", strings.NewReader("should not replace"))
	req.Header.Set("If-Match", `"not-current"`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT overwrite with If-Match mismatch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected If-Match mismatch to return 412, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/readme.md after failed conditional overwrite: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read content after failed conditional overwrite: %v", err)
	}
	if string(body) != "readme" {
		t.Fatalf("expected failed conditional overwrite to keep old content, got %q", body)
	}
}

func TestWebDAVPutNewFileMissingParentReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Missing/new.md", strings.NewReader("# New\n"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Missing/new.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected WebDAV PUT with missing parent to return 409, got %d", resp.StatusCode)
	}
}

func TestWebDAVPutToFolderReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes", strings.NewReader("not a folder"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected WebDAV PUT to folder to return 409, got %d", resp.StatusCode)
	}
}

func TestWebDAVPutContentRangeReturnsBadRequest(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/partial.md", strings.NewReader("partial"))
	req.Header.Set("Content-Range", "bytes 0-6/20")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/partial.md with Content-Range: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected WebDAV PUT with Content-Range to return 400, got %d", resp.StatusCode)
	}
}

func TestWebDAVPutOverMaxFileSizeReturnsTooLargeAndCleansTemp(t *testing.T) {
	app, _, tempDir, cleanup := newWebDAVLookupTestAppWithMaxFileSize(t, 5)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/too-large.md", strings.NewReader("123456"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/too-large.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized WebDAV PUT to return 413, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/too-large.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/too-large.md: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected oversized failed PUT target not to be visible, got %d", getResp.StatusCode)
	}
	webDAVTempDir := filepath.Join(tempDir, "webdav")
	entries, err := os.ReadDir(webDAVTempDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read WebDAV temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected oversized PUT to clean temp files, got %d entries", len(entries))
	}
}

func TestWebDAVPutOverQuotaReturnsInsufficientStorage(t *testing.T) {
	app, _, _, cleanup := newWebDAVLookupTestAppWithConfig(
		t,
		1024*1024,
		nil,
		func(cfg *config.Config) {
			cfg.Storage.QuotaBytes = 6
			cfg.Storage.TempLimitBytes = 1024
		},
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/quota.md", strings.NewReader("q"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/quota.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInsufficientStorage {
		t.Fatalf("expected over-quota WebDAV PUT to return 507, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/quota.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/quota.md: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected over-quota failed PUT target not to be visible, got %d", getResp.StatusCode)
	}
}

func TestWebDAVPutRechecksFullStagingCapacityAfterWritingTemp(t *testing.T) {
	app, _, tempDir, cleanup := newWebDAVLookupTestAppWithConfig(
		t,
		1024*1024,
		nil,
		func(cfg *config.Config) {
			cfg.Storage.QuotaBytes = 100
			cfg.Storage.TempLimitBytes = 9
		},
	)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPut,
		"/dav/Notes/staging.md",
		strings.NewReader("hello"),
	)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/staging.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInsufficientStorage {
		t.Fatalf("expected staging-limited WebDAV PUT to return 507, got %d", resp.StatusCode)
	}

	webDAVTempDir := filepath.Join(tempDir, "webdav")
	entries, err := os.ReadDir(webDAVTempDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read WebDAV temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected capacity failure to clean temp files, got %d entries", len(entries))
	}
}

func TestWebDAVPutKeepsFileWhenPipelineEnqueueFails(t *testing.T) {
	app, cleanup := newWebDAVLookupTestAppWithStoppedPipeline(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPut, "/dav/Notes/pipeline.md", strings.NewReader("pipeline still saves"))
	req.Header.Set("Content-Type", "text/markdown")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PUT /dav/Notes/pipeline.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected PUT to return 201 even when pipeline enqueue fails, got %d", resp.StatusCode)
	}
	if got := logs.String(); !strings.Contains(got, "event=pipeline_enqueue_failed") {
		t.Fatalf("expected pipeline enqueue failure to be logged, got %q", got)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/pipeline.md", nil))
	if err != nil {
		t.Fatalf("GET /dav/Notes/pipeline.md: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read pipeline failure saved file: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected file saved despite pipeline failure, got %d with body %s", getResp.StatusCode, string(body))
	}
	if string(body) != "pipeline still saves" {
		t.Fatalf("expected saved file body, got %q", body)
	}
}

func TestWebDAVMkcolCreatesFolder(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("MKCOL", "/dav/Notes/NewFolder", nil))
	if err != nil {
		t.Fatalf("MKCOL /dav/Notes/NewFolder: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected MKCOL to return 201, got %d", resp.StatusCode)
	}

	req := httptest.NewRequest("PROPFIND", "/dav/Notes/NewFolder", nil)
	req.Header.Set("Depth", "0")
	propfindResp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/NewFolder: %v", err)
	}
	defer propfindResp.Body.Close()
	body, err := io.ReadAll(propfindResp.Body)
	if err != nil {
		t.Fatalf("read created folder PROPFIND body: %v", err)
	}
	text := string(body)
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected created folder PROPFIND to return 207, got %d with body %s", propfindResp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:href>/dav/Notes/NewFolder/</D:href>`,
		`<D:displayname>NewFolder</D:displayname>`,
		`<D:resourcetype><D:collection></D:collection></D:resourcetype>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected created folder PROPFIND XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVMkcolTrailingSlashCreatesFolder(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("MKCOL", "/dav/Notes/Aaaa/", nil))
	if err != nil {
		t.Fatalf("MKCOL /dav/Notes/Aaaa/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected trailing-slash MKCOL to return 201, got %d", resp.StatusCode)
	}

	req := httptest.NewRequest("PROPFIND", "/dav/Notes/Aaaa/", nil)
	req.Header.Set("Depth", "0")
	propfindResp, err := app.Test(req)
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/Aaaa/: %v", err)
	}
	defer propfindResp.Body.Close()
	body, err := io.ReadAll(propfindResp.Body)
	if err != nil {
		t.Fatalf("read trailing-slash folder PROPFIND body: %v", err)
	}
	text := string(body)
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected trailing-slash folder PROPFIND to return 207, got %d with body %s", propfindResp.StatusCode, text)
	}
	for _, want := range []string{
		`<D:href>/dav/Notes/Aaaa/</D:href>`,
		`<D:displayname>Aaaa</D:displayname>`,
		`<D:resourcetype><D:collection></D:collection></D:resourcetype>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected trailing-slash folder PROPFIND XML to contain %q, got %s", want, text)
		}
	}
}

func TestWebDAVMkcolMissingParentReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("MKCOL", "/dav/Missing/NewFolder", nil))
	if err != nil {
		t.Fatalf("MKCOL /dav/Missing/NewFolder: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected MKCOL with missing parent to return 409, got %d", resp.StatusCode)
	}
}

func TestWebDAVMkcolExistingTargetReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	for _, path := range []string{"/dav/Notes", "/dav/Notes/readme.md"} {
		t.Run(path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("MKCOL", path, nil))
			if err != nil {
				t.Fatalf("MKCOL %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("expected MKCOL existing target %q to return 409, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestWebDAVMkcolCaseInsensitiveTargetConflictReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest("MKCOL", "/dav/notes", nil))
	if err != nil {
		t.Fatalf("MKCOL /dav/notes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected MKCOL case-insensitive target conflict to return 409, got %d", resp.StatusCode)
	}
}

func TestWebDAVMkcolRejectsUnsafeVirtualPath(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	for _, path := range []string{"/dav/.trash", "/dav/Notes//NewFolder"} {
		t.Run(path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("MKCOL", path, nil))
			if err != nil {
				t.Fatalf("MKCOL %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected unsafe MKCOL path %q to return 400, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestWebDAVDeleteFileSoftDeletesAndHidesFromWebDAV(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected WebDAV DELETE to return 204, got %d", resp.StatusCode)
	}

	propfindResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/readme.md after DELETE: %v", err)
	}
	defer propfindResp.Body.Close()
	if propfindResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected deleted file to disappear from WebDAV, got %d", propfindResp.StatusCode)
	}
}

func TestWebDAVDeleteFolderSoftDeletesDescendantsAndHidesFromWebDAV(t *testing.T) {
	app, db, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()
	createHandlerTestFile(t, db, t.TempDir(), &model.File{ID: "child", Name: "Child", Path: "/Notes", StoragePath: "Notes/Child", IsDir: true, Status: model.FileStatusReady}, "")
	createHandlerTestFile(t, db, t.TempDir(), &model.File{ID: "nested", Name: "nested.md", Path: "/Notes/Child", StoragePath: "Notes/Child/nested.md", Size: 6, MimeType: "text/markdown", Status: model.FileStatusReady}, "nested")

	resp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/dav/Notes/Child", nil))
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/Child: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected folder DELETE to return 204, got %d", resp.StatusCode)
	}

	for _, path := range []string{"/dav/Notes/Child", "/dav/Notes/Child/nested.md"} {
		t.Run(path, func(t *testing.T) {
			propfindResp, err := app.Test(httptest.NewRequest("PROPFIND", path, nil))
			if err != nil {
				t.Fatalf("PROPFIND %s after DELETE: %v", path, err)
			}
			defer propfindResp.Body.Close()
			if propfindResp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected deleted folder path %q to disappear from WebDAV, got %d", path, propfindResp.StatusCode)
			}
		})
	}
}

func TestWebDAVDeleteCreatesRestorableTrashEntry(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected WebDAV DELETE to return 204, got %d", resp.StatusCode)
	}

	trashResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/trash", nil))
	if err != nil {
		t.Fatalf("GET /trash after WebDAV DELETE: %v", err)
	}
	defer trashResp.Body.Close()
	if trashResp.StatusCode != http.StatusOK {
		t.Fatalf("expected trash list to return 200, got %d", trashResp.StatusCode)
	}
	var trashBody struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(trashResp.Body).Decode(&trashBody); err != nil {
		t.Fatalf("decode trash list: %v", err)
	}
	if len(trashBody.Files) != 1 || trashBody.Files[0].ID != "readme" {
		t.Fatalf("expected WebDAV-deleted file to appear in trash, got %#v", trashBody.Files)
	}

	restoreResp, err := app.Test(httptest.NewRequest(http.MethodPost, "/trash/readme/restore", nil))
	if err != nil {
		t.Fatalf("POST /trash/readme/restore: %v", err)
	}
	defer restoreResp.Body.Close()
	if restoreResp.StatusCode != http.StatusOK {
		t.Fatalf("expected trash restore to return 200, got %d", restoreResp.StatusCode)
	}

	propfindResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/readme.md after restore: %v", err)
	}
	defer propfindResp.Body.Close()
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected restored file to return to WebDAV view, got %d", propfindResp.StatusCode)
	}
}

func TestWebDAVDeleteIfMatchMismatchKeepsFile(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/dav/Notes/readme.md", nil)
	req.Header.Set("If-Match", `"not-current"`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("DELETE /dav/Notes/readme.md with mismatched If-Match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected mismatched If-Match DELETE to return 412, got %d", resp.StatusCode)
	}

	propfindResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav/Notes/readme.md after failed DELETE: %v", err)
	}
	defer propfindResp.Body.Close()
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected failed conditional DELETE to keep file visible, got %d", propfindResp.StatusCode)
	}
}

func TestWebDAVMoveFileRenamesAndKeepsFileID(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/renamed.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected WebDAV MOVE to return 201, got %d", resp.StatusCode)
	}

	oldResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND old path after MOVE: %v", err)
	}
	defer oldResp.Body.Close()
	if oldResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected old path to disappear after MOVE, got %d", oldResp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/renamed.md", nil))
	if err != nil {
		t.Fatalf("GET moved file: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "readme" {
		t.Fatalf("expected moved file to be readable, got %d with body %q", getResp.StatusCode, body)
	}

	fileResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/readme", nil))
	if err != nil {
		t.Fatalf("GET /files/readme after MOVE: %v", err)
	}
	defer fileResp.Body.Close()
	var file model.File
	if err := json.NewDecoder(fileResp.Body).Decode(&file); err != nil {
		t.Fatalf("decode moved file: %v", err)
	}
	if file.ID != "readme" || file.Name != "renamed.md" || file.Path != "/Notes" {
		t.Fatalf("expected File ID to be preserved with new location, got %#v", file)
	}
}

func TestWebDAVMoveFolderMovesDescendants(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	mkcolResp, err := app.Test(httptest.NewRequest("MKCOL", "/dav/Notes/Child", nil))
	if err != nil {
		t.Fatalf("MKCOL /dav/Notes/Child: %v", err)
	}
	defer mkcolResp.Body.Close()
	if mkcolResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected MKCOL to return 201, got %d", mkcolResp.StatusCode)
	}
	putResp, err := app.Test(httptest.NewRequest(http.MethodPut, "/dav/Notes/Child/nested.md", strings.NewReader("nested")))
	if err != nil {
		t.Fatalf("PUT nested file: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected nested PUT to return 201, got %d", putResp.StatusCode)
	}

	req := httptest.NewRequest("MOVE", "/dav/Notes/Child", nil)
	req.Header.Set("Destination", "http://example.com/dav/Moved")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE folder: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected folder MOVE to return 201, got %d", resp.StatusCode)
	}

	oldResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/Child/nested.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND old nested path: %v", err)
	}
	defer oldResp.Body.Close()
	if oldResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected old nested path to disappear, got %d", oldResp.StatusCode)
	}
	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Moved/nested.md", nil))
	if err != nil {
		t.Fatalf("GET moved nested file: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read moved nested file: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "nested" {
		t.Fatalf("expected moved nested file to be readable, got %d with body %q", getResp.StatusCode, body)
	}
}

func TestWebDAVMoveOverwriteFalseExistingTargetReturnsPreconditionFailed(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	putResp, err := app.Test(httptest.NewRequest(http.MethodPut, "/dav/Notes/source.md", strings.NewReader("source")))
	if err != nil {
		t.Fatalf("PUT source file: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected source PUT to return 201, got %d", putResp.StatusCode)
	}

	req := httptest.NewRequest("MOVE", "/dav/Notes/source.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/readme.md")
	req.Header.Set("Overwrite", "F")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE with Overwrite F: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected Overwrite F target conflict to return 412, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/source.md", nil))
	if err != nil {
		t.Fatalf("GET source after failed MOVE: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected source file to remain after failed MOVE, got %d", getResp.StatusCode)
	}
}

func TestWebDAVMoveOverExistingFileReplacesTargetByDefault(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	putResp, err := app.Test(httptest.NewRequest(http.MethodPut, "/dav/Notes/source.md", strings.NewReader("source")))
	if err != nil {
		t.Fatalf("PUT source file: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected source PUT to return 201, got %d", putResp.StatusCode)
	}

	req := httptest.NewRequest("MOVE", "/dav/Notes/source.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/readme.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE over existing file: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected overwrite MOVE to return 204, got %d", resp.StatusCode)
	}

	oldResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/source.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND source after overwrite MOVE: %v", err)
	}
	defer oldResp.Body.Close()
	if oldResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected source path to disappear after overwrite MOVE, got %d", oldResp.StatusCode)
	}
	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET overwritten target: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read overwritten target: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "source" {
		t.Fatalf("expected target to contain moved source body, got %d with body %q", getResp.StatusCode, body)
	}
}

func TestWebDAVMoveMissingDestinationParentReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Missing/renamed.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE to missing parent: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected MOVE to missing parent to return 409, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET source after failed parent MOVE: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected source to remain after missing parent MOVE, got %d", getResp.StatusCode)
	}
}

func TestWebDAVMoveFolderIntoOwnSubtreeReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/Sub")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE folder into own subtree: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected MOVE folder into own subtree to return 409, got %d", resp.StatusCode)
	}

	propfindResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND source child after failed subtree MOVE: %v", err)
	}
	defer propfindResp.Body.Close()
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected source tree to remain after failed subtree MOVE, got %d", propfindResp.StatusCode)
	}
}

func TestWebDAVMoveRejectsDestinationOutsideDavEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name        string
		destination string
	}{
		{name: "cross host", destination: "http://other.example/dav/Notes/renamed.md"},
		{name: "cross scheme", destination: "https://example.com/dav/Notes/renamed.md"},
		{name: "non dav path", destination: "http://example.com/files/renamed.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _, cleanup := newWebDAVLookupTestApp(t)
			defer cleanup()

			req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
			req.Header.Set("Destination", tc.destination)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("MOVE with %s destination: %v", tc.name, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected invalid Destination %q to return 400, got %d", tc.destination, resp.StatusCode)
			}
		})
	}
}

func TestWebDAVMoveAcceptsHttpsDestinationBehindReverseProxy(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Destination", "https://example.com/dav/Notes/renamed.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE /dav/Notes/readme.md behind HTTPS proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected WebDAV MOVE behind HTTPS proxy to return 201, got %d", resp.StatusCode)
	}
}

func TestWebDAVMoveAcceptsHttpsDestinationWhenTLSIsTerminatedOnDefaultPort(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Host = "memodrive.tail6f3b17.ts.net:443"
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("Destination", "https://memodrive.tail6f3b17.ts.net:443/dav/Notes/renamed.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE behind TLS-terminated proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected WebDAV MOVE behind TLS-terminated proxy to return 201, got %d", resp.StatusCode)
	}
}

func TestWebDAVMoveAcceptsRootDestinationWithoutMountPrefix(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Host = "memodrive.tail6f3b17.ts.net:443"
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("Destination", "https://memodrive.tail6f3b17.ts.net:443/renamed-root.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE to root without mount prefix: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected WebDAV MOVE to root without mount prefix to return 201, got %d", resp.StatusCode)
	}

	oldResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND old path after root MOVE: %v", err)
	}
	defer oldResp.Body.Close()
	if oldResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected old path to disappear after root MOVE, got %d", oldResp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/renamed-root.md", nil))
	if err != nil {
		t.Fatalf("GET root moved file: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read root moved file: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "readme" {
		t.Fatalf("expected root moved file to be readable, got %d with body %q", getResp.StatusCode, body)
	}
}

func TestWebDAVMoveAllowsCaseOnlyRename(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/README.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE case-only rename: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected case-only MOVE to return 201, got %d", resp.StatusCode)
	}

	propfindResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/README.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND case-renamed file: %v", err)
	}
	defer propfindResp.Body.Close()
	body, err := io.ReadAll(propfindResp.Body)
	if err != nil {
		t.Fatalf("read case-renamed PROPFIND body: %v", err)
	}
	text := string(body)
	if propfindResp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected case-renamed file to be visible, got %d with body %s", propfindResp.StatusCode, text)
	}
	if !strings.Contains(text, `<D:displayname>README.md</D:displayname>`) {
		t.Fatalf("expected displayname to use new casing, got %s", text)
	}
}

func TestWebDAVMoveIfMatchMismatchKeepsSource(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("MOVE", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/renamed.md")
	req.Header.Set("If-Match", `"not-current"`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("MOVE with mismatched If-Match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected mismatched If-Match MOVE to return 412, got %d", resp.StatusCode)
	}

	sourceResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET source after failed conditional MOVE: %v", err)
	}
	defer sourceResp.Body.Close()
	if sourceResp.StatusCode != http.StatusOK {
		t.Fatalf("expected source to remain after failed conditional MOVE, got %d", sourceResp.StatusCode)
	}
	destResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/renamed.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND destination after failed conditional MOVE: %v", err)
	}
	defer destResp.Body.Close()
	if destResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected destination not to exist after failed conditional MOVE, got %d", destResp.StatusCode)
	}
}

func TestWebDAVCopyFileCreatesNewFileAndKeepsSource(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("COPY", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/copy.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY /dav/Notes/readme.md: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected WebDAV COPY to return 201, got %d", resp.StatusCode)
	}

	for _, path := range []string{"/dav/Notes/readme.md", "/dav/Notes/copy.md"} {
		t.Run(path, func(t *testing.T) {
			getResp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
			if err != nil {
				t.Fatalf("GET %s after COPY: %v", path, err)
			}
			defer getResp.Body.Close()
			body, err := io.ReadAll(getResp.Body)
			if err != nil {
				t.Fatalf("read %s after COPY: %v", path, err)
			}
			if getResp.StatusCode != http.StatusOK || string(body) != "readme" {
				t.Fatalf("expected %s to be readable with copied body, got %d with body %q", path, getResp.StatusCode, body)
			}
		})
	}

	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files?path=/Notes", nil))
	if err != nil {
		t.Fatalf("GET /files?path=/Notes: %v", err)
	}
	defer listResp.Body.Close()
	var listBody struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode file list: %v", err)
	}
	var copyFile *model.File
	for i := range listBody.Files {
		if listBody.Files[i].Name == "copy.md" {
			copyFile = &listBody.Files[i]
		}
	}
	if copyFile == nil {
		t.Fatalf("expected copied file to appear in /files list, got %#v", listBody.Files)
	}
	if copyFile.ID == "readme" {
		t.Fatalf("expected copied file to have a new File ID, got source ID %q", copyFile.ID)
	}
}

func TestWebDAVCopyAcceptsHttpsDestinationWhenTLSIsTerminatedOnDefaultPort(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("COPY", "/dav/Notes/readme.md", nil)
	req.Host = "memodrive.tail6f3b17.ts.net:443"
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("Destination", "https://memodrive.tail6f3b17.ts.net:443/dav/Notes/copy.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY behind TLS-terminated proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected WebDAV COPY behind TLS-terminated proxy to return 201, got %d", resp.StatusCode)
	}
}

func TestWebDAVCopyOverExistingFileKeepsTargetFileID(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	putResp, err := app.Test(httptest.NewRequest(http.MethodPut, "/dav/Notes/source.md", strings.NewReader("source")))
	if err != nil {
		t.Fatalf("PUT source file: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected source PUT to return 201, got %d", putResp.StatusCode)
	}

	req := httptest.NewRequest("COPY", "/dav/Notes/source.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/readme.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY over existing target: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected overwrite COPY to return 204, got %d", resp.StatusCode)
	}

	targetResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET overwritten copy target: %v", err)
	}
	defer targetResp.Body.Close()
	body, err := io.ReadAll(targetResp.Body)
	if err != nil {
		t.Fatalf("read overwritten copy target: %v", err)
	}
	if targetResp.StatusCode != http.StatusOK || string(body) != "source" {
		t.Fatalf("expected target body to be copied source, got %d with body %q", targetResp.StatusCode, body)
	}

	sourceResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/source.md", nil))
	if err != nil {
		t.Fatalf("GET copy source after overwrite: %v", err)
	}
	defer sourceResp.Body.Close()
	if sourceResp.StatusCode != http.StatusOK {
		t.Fatalf("expected copy source to remain visible, got %d", sourceResp.StatusCode)
	}

	fileResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/readme", nil))
	if err != nil {
		t.Fatalf("GET /files/readme after overwrite COPY: %v", err)
	}
	defer fileResp.Body.Close()
	var file model.File
	if err := json.NewDecoder(fileResp.Body).Decode(&file); err != nil {
		t.Fatalf("decode overwritten target file: %v", err)
	}
	if file.ID != "readme" || file.Name != "readme.md" || file.Path != "/Notes" || file.Size != 6 {
		t.Fatalf("expected target File ID and location to be preserved, got %#v", file)
	}
}

func TestWebDAVCopyFolderRecursivelyCopiesChildren(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("COPY", "/dav/Notes", nil)
	req.Header.Set("Destination", "http://example.com/dav/NotesCopy")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY folder: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected folder COPY to return 201, got %d", resp.StatusCode)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/NotesCopy/readme.md", nil))
	if err != nil {
		t.Fatalf("GET copied folder child: %v", err)
	}
	defer getResp.Body.Close()
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read copied folder child: %v", err)
	}
	if getResp.StatusCode != http.StatusOK || string(body) != "readme" {
		t.Fatalf("copied folder child status = %d, body = %q", getResp.StatusCode, body)
	}
}

func TestWebDAVCopyToFolderReturnsConflict(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("COPY", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY to folder target: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected COPY to folder target to return 409, got %d", resp.StatusCode)
	}
}

func TestWebDAVCopyOverwriteFalseExistingTargetReturnsPreconditionFailed(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	putResp, err := app.Test(httptest.NewRequest(http.MethodPut, "/dav/Notes/source.md", strings.NewReader("source")))
	if err != nil {
		t.Fatalf("PUT source file: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected source PUT to return 201, got %d", putResp.StatusCode)
	}

	req := httptest.NewRequest("COPY", "/dav/Notes/source.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/readme.md")
	req.Header.Set("Overwrite", "F")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY with Overwrite F: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected Overwrite F COPY conflict to return 412, got %d", resp.StatusCode)
	}

	targetResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET target after failed COPY: %v", err)
	}
	defer targetResp.Body.Close()
	body, err := io.ReadAll(targetResp.Body)
	if err != nil {
		t.Fatalf("read target after failed COPY: %v", err)
	}
	if targetResp.StatusCode != http.StatusOK || string(body) != "readme" {
		t.Fatalf("expected target body to remain unchanged, got %d with body %q", targetResp.StatusCode, body)
	}
}

func TestWebDAVCopyAttemptsPipelineEnqueue(t *testing.T) {
	app, cleanup := newWebDAVLookupTestAppWithStoppedPipeline(t)
	defer cleanup()

	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest("COPY", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/copy.md")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY with stopped pipeline: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected COPY to return 201 even when pipeline enqueue fails, got %d", resp.StatusCode)
	}
	if got := logs.String(); !strings.Contains(got, "event=pipeline_enqueue_failed") {
		t.Fatalf("expected COPY to attempt pipeline enqueue and log failure, got %q", got)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/copy.md", nil))
	if err != nil {
		t.Fatalf("GET copied file after pipeline failure: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected copied file to remain saved despite pipeline failure, got %d", getResp.StatusCode)
	}
}

func TestWebDAVCopyIfMatchMismatchDoesNotCreateTarget(t *testing.T) {
	app, _, cleanup := newWebDAVLookupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("COPY", "/dav/Notes/readme.md", nil)
	req.Header.Set("Destination", "http://example.com/dav/Notes/copy.md")
	req.Header.Set("If-Match", `"not-current"`)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("COPY with mismatched If-Match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected mismatched If-Match COPY to return 412, got %d", resp.StatusCode)
	}

	destResp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav/Notes/copy.md", nil))
	if err != nil {
		t.Fatalf("PROPFIND copy destination after failed conditional COPY: %v", err)
	}
	defer destResp.Body.Close()
	if destResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected destination not to exist after failed conditional COPY, got %d", destResp.StatusCode)
	}
	sourceResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/Notes/readme.md", nil))
	if err != nil {
		t.Fatalf("GET source after failed conditional COPY: %v", err)
	}
	defer sourceResp.Body.Close()
	if sourceResp.StatusCode != http.StatusOK {
		t.Fatalf("expected source to remain after failed conditional COPY, got %d", sourceResp.StatusCode)
	}
}

func webDAVTestPropertyValue(t *testing.T, body, name string) string {
	t.Helper()
	open := "<D:" + name + ">"
	close := "</D:" + name + ">"
	start := strings.Index(body, open)
	if start < 0 {
		t.Fatalf("expected WebDAV XML to contain %s in %s", open, body)
	}
	start += len(open)
	end := strings.Index(body[start:], close)
	if end < 0 {
		t.Fatalf("expected WebDAV XML to contain %s in %s", close, body)
	}
	return body[start : start+end]
}

func webDAVTestPropstatWithStatus(t *testing.T, body, status string) string {
	t.Helper()
	rest := body
	for {
		start := strings.Index(rest, "<D:propstat>")
		if start < 0 {
			t.Fatalf("expected propstat with status %q in %s", status, body)
		}
		end := strings.Index(rest[start:], "</D:propstat>")
		if end < 0 {
			t.Fatalf("expected closed propstat with status %q in %s", status, body)
		}
		section := rest[start : start+end+len("</D:propstat>")]
		if strings.Contains(section, "<D:status>"+status+"</D:status>") {
			return section
		}
		rest = rest[start+len("<D:propstat>"):]
	}
}

func assertAllowHeader(t *testing.T, header string, want []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, part := range strings.Split(header, ",") {
		method := strings.TrimSpace(part)
		if method != "" {
			seen[method] = true
		}
	}
	for _, method := range want {
		if !seen[method] {
			t.Fatalf("expected Allow header %q to contain %s", header, method)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("expected Allow header to contain exactly %v, got %q", want, header)
	}
}

func newWebDAVLookupTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	app, db, _, cleanup := newWebDAVLookupTestAppWithMaxFileSize(t, 1024*1024)
	return app, db, cleanup
}

func newWebDAVIntegrationTestApp(t *testing.T) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{
		Auth:   config.AuthConfig{Password: "secret"},
		WebDAV: config.WebDAVConfig{Enabled: true},
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  1024 * 1024,
			ChunkSize:    5,
		},
		Pipeline: config.PipelineConfig{
			Workers:         1,
			ChunkSize:       500,
			ChunkOverlap:    100,
			EmbedBatchSize:  1,
			ParentChunkSize: 1024,
			ChildChunkSize:  256,
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "readme", Name: "readme.md", Path: "/Notes", StoragePath: "Notes/readme.md", Size: 6, MimeType: "text/markdown", Status: model.FileStatusReady}, "readme")
	pipeline := service.NewPipelineService(cfg, db, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pipeline.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown pipeline stub: %v", err)
	}
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, cfg, service.NewWebDAVService(cfg, db, pipeline))
	NewTrashHandler(service.NewFileService(cfg, db, nil)).Register(app)
	return app, func() {
		_ = db.Close()
	}
}

func webDAVIntegrationRequest(t *testing.T, app *fiber.App, method, target string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	req.SetBasicAuth("admin", "secret")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	return resp
}

func readWebDAVIntegrationBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}

func newWebDAVLookupTestAppWithMaxFileSize(t *testing.T, maxFileSize int64) (*fiber.App, *store.Store, string, func()) {
	return newWebDAVLookupTestAppWithMaxFileSizeAndPipeline(t, maxFileSize, nil)
}

func newWebDAVLookupTestAppWithStoppedPipeline(t *testing.T) (*fiber.App, func()) {
	var pipeline *service.PipelineService
	app, _, _, cleanup := newWebDAVLookupTestAppWithMaxFileSizeAndPipeline(t, 1024*1024, &pipeline)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pipeline.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown pipeline: %v", err)
	}
	return app, cleanup
}

func newWebDAVLookupTestAppWithVectorStore(t *testing.T, vector vectordb.VectorStore) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  1024 * 1024,
			ChunkSize:    5,
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "readme", Name: "readme.md", Path: "/Notes", StoragePath: "Notes/readme.md", Size: 6, MimeType: "text/markdown", Status: model.FileStatusReady}, "readme")
	pipeline := service.NewPipelineService(cfg, db, nil, vector, nil, nil)
	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	RegisterWebDAV(app, cfg, service.NewWebDAVService(cfg, db, pipeline))
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	NewTrashHandler(service.NewFileService(cfg, db, nil)).Register(app)
	return app, db, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pipeline.Shutdown(ctx)
		_ = db.Close()
	}
}

func newWebDAVLookupTestAppWithMaxFileSizeAndPipeline(t *testing.T, maxFileSize int64, pipelineOut **service.PipelineService) (*fiber.App, *store.Store, string, func()) {
	return newWebDAVLookupTestAppWithConfig(t, maxFileSize, pipelineOut, nil)
}

func newWebDAVLookupTestAppWithConfig(
	t *testing.T,
	maxFileSize int64,
	pipelineOut **service.PipelineService,
	configure func(*config.Config),
) (*fiber.App, *store.Store, string, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	tempDir := filepath.Join(root, "tmp")
	cfg := &config.Config{
		WebDAV: config.WebDAVConfig{Enabled: true},
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      tempDir,
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  maxFileSize,
			ChunkSize:    5,
		},
	}
	if configure != nil {
		configure(cfg)
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "readme", Name: "readme.md", Path: "/Notes", StoragePath: "Notes/readme.md", Size: 6, MimeType: "text/markdown", Status: model.FileStatusReady}, "readme")

	app := fiber.New(fiber.Config{RequestMethods: WebDAVRequestMethods(fiber.DefaultMethods)})
	var pipeline *service.PipelineService
	if pipelineOut != nil {
		pipeline = service.NewPipelineService(cfg, db, nil, nil, nil, nil)
		*pipelineOut = pipeline
	}
	RegisterWebDAV(app, cfg, service.NewWebDAVService(cfg, db, pipeline))
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	NewTrashHandler(service.NewFileService(cfg, db, nil)).Register(app)
	return app, db, tempDir, func() {
		if pipeline != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = pipeline.Shutdown(ctx)
		}
		_ = db.Close()
	}
}

type webDAVRecordingVectorStore struct {
	deletedIDs []string
}

func (s *webDAVRecordingVectorStore) EnsureCollection(context.Context, string) error {
	return nil
}

func (s *webDAVRecordingVectorStore) Upsert(context.Context, string, []string, [][]float32, []string, []map[string]any) error {
	return nil
}

func (s *webDAVRecordingVectorStore) Query(context.Context, string, []float32, int) (*vectordb.QueryResult, error) {
	return nil, nil
}

func (s *webDAVRecordingVectorStore) Delete(_ context.Context, _ string, ids []string) error {
	s.deletedIDs = append(s.deletedIDs, ids...)
	return nil
}
