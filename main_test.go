// Integration test for XordPress Phase 1 (design.md §8, tasks.md T24).
//
// This is a black-box test: it builds the real binary once, launches it as
// a subprocess against a fresh t.TempDir() data directory and an ephemeral
// port, and drives it purely over HTTP -- exactly as a real client would.
// It reads the subprocess's stdout for the deterministic "listening on ..."
// and "setup token: ..." lines instead of sleep-based polling (design.md
// §8).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// buildIntegrationBinary builds the xordpress binary once for the whole
// test, instrumented for coverage (go build -cover) across all internal
// packages, per design.md §8 / tasks.md T24 ("カバレッジは go build -cover
// + GOCOVERDIR で計測する").
func buildIntegrationBinary(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	binPath := binDir + "/xordpress-under-test"

	cmd := exec.Command("go", "build", "-cover", "-coverpkg=./...", "-o", binPath, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build instrumented binary: %v\n%s", err, out)
	}
	return binPath
}

// mergeAndReportSubprocessCoverage converts the GOCOVERDIR counters written
// by the xordpress subprocess(es) into a real coverage report, so that data
// feeds a gate instead of being silently discarded when covDir is removed
// (tasks.md T24: "サブプロセスとして起動したバイナリのカバレッジもCIゲート
// に含める"). Must run only after every subprocess sharing covDir has
// exited, since counters are flushed on process exit.
func mergeAndReportSubprocessCoverage(t *testing.T, covDir string) {
	t.Helper()

	percentOut, err := exec.Command("go", "tool", "covdata", "percent", "-i="+covDir).CombinedOutput()
	if err != nil {
		t.Fatalf("compute subprocess coverage percent: %v\n%s", err, percentOut)
	}
	t.Logf("subprocess (GOCOVERDIR) coverage by package:\n%s", percentOut)
	if !strings.Contains(string(percentOut), "internal/") {
		t.Fatalf("expected subprocess coverage to cover internal/* packages, got:\n%s", percentOut)
	}

	textDir, err := os.MkdirTemp("", "xordpress-integration-coverage")
	if err != nil {
		t.Fatalf("create coverage textfmt dir: %v", err)
	}
	defer os.RemoveAll(textDir)

	mergedPath := filepath.Join(textDir, "integration-coverage.txt")
	textOut, err := exec.Command("go", "tool", "covdata", "textfmt", "-i="+covDir, "-o="+mergedPath).CombinedOutput()
	if err != nil {
		t.Fatalf("convert subprocess coverage to textfmt: %v\n%s", err, textOut)
	}
	data, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("read merged coverage: %v", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		t.Fatalf("merged subprocess coverage output is empty")
	}
}

// testServer represents a running xordpress subprocess.
type testServer struct {
	t          *testing.T
	cmd        *exec.Cmd
	baseURL    string // actual address the process is listening on, e.g. "http://127.0.0.1:54321"
	setupToken string // "" if the process did not print one (users already existed)
	stderr     *bytes.Buffer
	stdoutDone chan struct{}
	stopOnce   sync.Once
}

// startServer launches the built binary against dataDir with a fixed
// --base-url (used for CSRF Origin comparisons, deliberately independent of
// the actual bind address) and an ephemeral --addr. It blocks until the
// subprocess prints "listening on ..." (or "setup token: ..." first, if a
// setup token was generated), per design.md §8's guidance to read
// deterministic startup output rather than poll with sleep.
func startServer(t *testing.T, binPath, dataDir, covDir string) *testServer {
	t.Helper()

	cmd := exec.Command(binPath,
		"--addr", "127.0.0.1:0",
		"--data-dir", dataDir,
		"--base-url", "http://xordpress.test",
	)
	// GOCOVERDIR: the binary was built with `go build -cover`, so it writes
	// coverage counter files to this directory on a clean exit (tasks.md
	// T24's coverage requirement covers the subprocess, not just the test
	// binary itself).
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+covDir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	srv := &testServer{
		t:          t,
		cmd:        cmd,
		stderr:     &stderrBuf,
		stdoutDone: make(chan struct{}),
	}

	scanner := bufio.NewScanner(stdout)
	var addr string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "setup token: ") {
			srv.setupToken = strings.TrimPrefix(line, "setup token: ")
			continue
		}
		if strings.HasPrefix(line, "listening on ") {
			addr = strings.TrimPrefix(line, "listening on ")
			break
		}
	}
	if addr == "" {
		_ = cmd.Process.Kill()
		t.Fatalf("server never printed a listening line; stderr=%s", stderrBuf.String())
	}
	srv.baseURL = "http://" + addr

	// Keep draining stdout in the background so the child never blocks on
	// a full pipe once the test stops reading lines synchronously.
	go func() {
		defer close(srv.stdoutDone)
		for scanner.Scan() {
		}
	}()

	return srv
}

// stop sends SIGTERM (handled by signal.NotifyContext in main.go) so the
// process shuts down gracefully and its GOCOVERDIR coverage counters are
// flushed on normal exit -- os.Process.Kill (SIGKILL) would discard them.
func (s *testServer) stop() {
	s.t.Helper()
	s.stopOnce.Do(func() {
		if s.cmd.Process == nil {
			return
		}
		_ = s.cmd.Process.Signal(syscall.SIGTERM)

		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()

		select {
		case err := <-done:
			if err != nil {
				s.t.Logf("server exited with error: %v (stderr=%s)", err, s.stderr.String())
			}
		case <-time.After(10 * time.Second):
			s.t.Logf("server did not exit within 10s of SIGTERM; killing")
			_ = s.cmd.Process.Kill()
			<-done
		}
		<-s.stdoutDone
	})
}

// apiClient is a minimal manual HTTP client that tracks the session cookie
// explicitly (rather than relying on http.CookieJar's host-only matching,
// which would blur together servers on different ports of 127.0.0.1 across
// scenario 14's restart).
type apiClient struct {
	baseURL string
	cookie  string // full "name=value" Cookie header value, "" if unauthenticated
}

type apiResponse struct {
	status int
	header http.Header
	body   []byte
}

func (c *apiClient) do(t *testing.T, method, path string, body []byte, headers map[string]string) apiResponse {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return apiResponse{status: resp.StatusCode, header: resp.Header, body: respBody}
}

// login performs POST /api/session and stores the resulting session cookie
// on the client for subsequent authenticated requests.
func (c *apiClient) login(t *testing.T, username, password string) apiResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp := c.do(t, "POST", "/api/session", body, map[string]string{"Content-Type": "application/json"})
	if resp.status == http.StatusNoContent {
		for _, sc := range resp.header.Values("Set-Cookie") {
			if strings.HasPrefix(sc, "xordpress_session=") {
				c.cookie = strings.SplitN(sc, ";", 2)[0]
			}
		}
	}
	return resp
}

func jsonBody(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func decodeJSON(t *testing.T, resp apiResponse, v any) {
	t.Helper()
	if err := json.Unmarshal(resp.body, v); err != nil {
		t.Fatalf("decode JSON (status=%d body=%s): %v", resp.status, resp.body, err)
	}
}

const (
	adminUsername = "admin"
	adminPassword = "correct-password-123"
	siteNameInit  = "Test Site"
)

// TestIntegration drives design.md §8's 15 black-box scenarios against a
// real subprocess, in order, sharing state as the scenarios themselves
// depend on it (e.g. scenario 5's version bump feeds scenario 7's stale
// If-Match check).
func TestIntegration(t *testing.T) {
	binPath := buildIntegrationBinary(t)
	dataDir := t.TempDir()
	covDir := t.TempDir()
	// Registered before srv/srv2's stop cleanups (line ~247, ~538 below), so
	// by LIFO cleanup ordering it fires *after* both subprocesses have
	// stopped (and flushed their GOCOVERDIR counters) but *before* covDir's
	// own t.TempDir() cleanup deletes those counters out from under it.
	t.Cleanup(func() { mergeAndReportSubprocessCoverage(t, covDir) })

	srv := startServer(t, binPath, dataDir, covDir)
	t.Cleanup(srv.stop)

	unauth := &apiClient{baseURL: srv.baseURL}
	admin := &apiClient{baseURL: srv.baseURL}

	origin := map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json"}

	var postID float64
	var postSlug = "hello-world"

	// --- Scenario 1: setup token handling ---
	t.Run("scenario1_setup_token", func(t *testing.T) {
		if srv.setupToken == "" {
			t.Fatal("server did not print a setup token on first boot")
		}

		// Correct token, with site_name -> 201.
		resp := unauth.do(t, "POST", "/api/setup",
			jsonBody(t, map[string]string{"username": adminUsername, "password": adminPassword, "site_name": siteNameInit}),
			map[string]string{"Authorization": "Bearer " + srv.setupToken, "Content-Type": "application/json"})
		if resp.status != http.StatusCreated {
			t.Fatalf("setup with correct token: got %d, body=%s", resp.status, resp.body)
		}

		// Wrong token -> 401.
		resp = unauth.do(t, "POST", "/api/setup",
			jsonBody(t, map[string]string{"username": "someoneelse", "password": adminPassword, "site_name": siteNameInit}),
			map[string]string{"Authorization": "Bearer wrong-token", "Content-Type": "application/json"})
		if resp.status != http.StatusUnauthorized {
			t.Fatalf("setup with wrong token: got %d, body=%s", resp.status, resp.body)
		}

		// Same correct token again, same process: the token itself still
		// compares equal (it is not rotated mid-process -- design.md §4
		// line 175), but content.CreateFirstAdmin now finds a user already
		// exists and reports created=false, which adminapi maps to 409.
		// design.md §8 scenario 1's "正しいトークンで再実行→401" describes
		// the token no longer being valid *because it was never reissued*
		// -- that statement is only true across a restart (no user existed
		// yet at token-generation time is no longer true, so no token is
		// generated at all: see §4 line 180). That restart case is
		// exercised in scenario 14 below, where the same original token is
		// replayed against the restarted process and does get 401.
		resp = unauth.do(t, "POST", "/api/setup",
			jsonBody(t, map[string]string{"username": "someoneelse2", "password": adminPassword, "site_name": siteNameInit}),
			map[string]string{"Authorization": "Bearer " + srv.setupToken, "Content-Type": "application/json"})
		if resp.status != http.StatusConflict {
			t.Fatalf("setup replaying correct token same-process: got %d (want 409), body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 2: login ---
	t.Run("scenario2_login", func(t *testing.T) {
		resp := admin.login(t, adminUsername, adminPassword)
		if resp.status != http.StatusNoContent {
			t.Fatalf("login: got %d, body=%s", resp.status, resp.body)
		}
		if admin.cookie == "" {
			t.Fatal("login did not set a session cookie")
		}

		wrong := &apiClient{baseURL: srv.baseURL}
		resp = wrong.login(t, adminUsername, "wrong-password")
		if resp.status != http.StatusUnauthorized {
			t.Fatalf("login with wrong password: got %d, body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 3: create draft post ---
	t.Run("scenario3_create_draft", func(t *testing.T) {
		headers := map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json"}
		resp := admin.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "Hello World", "slug": postSlug, "body": "<p>Hello, world.</p>", "status": "draft",
		}), headers)
		if resp.status != http.StatusCreated {
			t.Fatalf("create draft: got %d, body=%s", resp.status, resp.body)
		}
		var out map[string]any
		decodeJSON(t, resp, &out)
		v, ok := out["version"].(float64)
		if !ok || v != 1 {
			t.Fatalf("expected version 1 in create response, got %v", out["version"])
		}
		id, ok := out["id"].(float64)
		if !ok {
			t.Fatalf("expected numeric id in create response, got %v", out["id"])
		}
		postID = id
	})

	// --- Scenario 4: draft not publicly visible ---
	t.Run("scenario4_draft_not_public", func(t *testing.T) {
		resp := unauth.do(t, "GET", "/"+postSlug, nil, nil)
		if resp.status != http.StatusNotFound {
			t.Fatalf("GET /%s for draft post: got %d (want 404), body=%s", postSlug, resp.status, resp.body)
		}
	})

	// --- Scenario 5: publish via PUT with correct If-Match ---
	t.Run("scenario5_publish", func(t *testing.T) {
		headers := map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json", "If-Match": `"1"`}
		resp := admin.do(t, "PUT", fmt.Sprintf("/api/posts/%d", int64(postID)), jsonBody(t, map[string]string{
			"title": "Hello World", "slug": postSlug, "body": "<p>Hello, world.</p>", "status": "published",
		}), headers)
		if resp.status != http.StatusOK {
			t.Fatalf("publish: got %d, body=%s", resp.status, resp.body)
		}
		var out map[string]any
		decodeJSON(t, resp, &out)
		if v, ok := out["version"].(float64); !ok || v != 2 {
			t.Fatalf("expected version 2 after publish, got %v", out["version"])
		}
	})

	// --- Scenario 6: published post visible publicly, with site_name rendered ---
	t.Run("scenario6_public_view", func(t *testing.T) {
		resp := unauth.do(t, "GET", "/"+postSlug, nil, nil)
		if resp.status != http.StatusOK {
			t.Fatalf("GET /%s published: got %d, body=%s", postSlug, resp.status, resp.body)
		}
		html := string(resp.body)
		if !strings.Contains(html, "Hello World") {
			t.Errorf("rendered HTML missing post title: %s", html)
		}
		if !strings.Contains(html, "Hello, world.") {
			t.Errorf("rendered HTML missing post body: %s", html)
		}
		if !strings.Contains(html, siteNameInit) {
			t.Errorf("rendered HTML missing site_name %q: %s", siteNameInit, html)
		}
	})

	// --- Scenario 7: optimistic concurrency ---
	t.Run("scenario7_optimistic_concurrency", func(t *testing.T) {
		headers := map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json", "If-Match": `"1"`}
		resp := admin.do(t, "PUT", fmt.Sprintf("/api/posts/%d", int64(postID)), jsonBody(t, map[string]string{
			"title": "Hello World", "slug": postSlug, "body": "<p>Hello, world.</p>", "status": "published",
		}), headers)
		if resp.status != http.StatusPreconditionFailed {
			t.Fatalf("stale If-Match: got %d (want 412), body=%s", resp.status, resp.body)
		}

		headersNoIfMatch := map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json"}
		resp = admin.do(t, "PUT", fmt.Sprintf("/api/posts/%d", int64(postID)), jsonBody(t, map[string]string{
			"title": "Hello World", "slug": postSlug, "body": "<p>Hello, world.</p>", "status": "published",
		}), headersNoIfMatch)
		if resp.status != http.StatusBadRequest {
			t.Fatalf("missing If-Match: got %d (want 400), body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 8: slug conflicts ---
	t.Run("scenario8_slug_conflicts", func(t *testing.T) {
		resp := admin.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "Duplicate", "slug": postSlug, "body": "<p>dup</p>", "status": "draft",
		}), origin)
		if resp.status != http.StatusConflict {
			t.Fatalf("duplicate slug: got %d (want 409), body=%s", resp.status, resp.body)
		}

		resp = admin.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "Reserved", "slug": "api", "body": "<p>reserved</p>", "status": "draft",
		}), origin)
		if resp.status != http.StatusBadRequest {
			t.Fatalf("reserved slug: got %d (want 400), body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 9: CSRF Origin + Content-Type/DELETE consistency ---
	var originTestPostID float64
	t.Run("scenario9_csrf_and_content_type", func(t *testing.T) {
		badOrigin := map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}
		resp := admin.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "Origin Test", "slug": "origin-test", "body": "<p>o</p>", "status": "draft",
		}), badOrigin)
		if resp.status != http.StatusForbidden {
			t.Fatalf("bad Origin: got %d (want 403), body=%s", resp.status, resp.body)
		}

		noOrigin := map[string]string{"Content-Type": "application/json"}
		resp = admin.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "Origin Test", "slug": "origin-test", "body": "<p>o</p>", "status": "draft",
		}), noOrigin)
		if resp.status != http.StatusCreated {
			t.Fatalf("no Origin header should be allowed: got %d, body=%s", resp.status, resp.body)
		}
		var out map[string]any
		decodeJSON(t, resp, &out)
		id, ok := out["id"].(float64)
		if !ok {
			t.Fatalf("expected numeric id, got %v", out["id"])
		}
		originTestPostID = id

		// DELETE /api/session without Content-Type set must not be
		// rejected by the Content-Type check (design.md §4: it only
		// applies to POST/PUT).
		resp = admin.do(t, "DELETE", "/api/session", nil, nil)
		if resp.status != http.StatusNoContent {
			t.Fatalf("logout without Content-Type: got %d (want 204), body=%s", resp.status, resp.body)
		}
		admin.cookie = "" // logout invalidated the session; re-login below.
	})

	// Re-authenticate: scenario 9 logged the admin client out.
	t.Run("relogin_after_logout", func(t *testing.T) {
		resp := admin.login(t, adminUsername, adminPassword)
		if resp.status != http.StatusNoContent {
			t.Fatalf("re-login after logout: got %d, body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 10: list envelope ---
	t.Run("scenario10_list_envelope", func(t *testing.T) {
		resp := admin.do(t, "GET", "/api/posts", nil, nil)
		if resp.status != http.StatusOK {
			t.Fatalf("list posts: got %d, body=%s", resp.status, resp.body)
		}
		var out struct {
			Items []map[string]any `json:"items"`
			Total float64          `json:"total"`
		}
		decodeJSON(t, resp, &out)
		if out.Total <= 0 {
			t.Fatalf("expected total > 0, got %v", out.Total)
		}
		if len(out.Items) == 0 {
			t.Fatal("expected at least one item")
		}
		for _, item := range out.Items {
			if _, ok := item["version"]; !ok {
				t.Fatalf("list item missing version field: %v", item)
			}
		}
	})

	// --- Scenario 11: asset serving + percent-encoded traversal ---
	t.Run("scenario11_assets", func(t *testing.T) {
		resp := unauth.do(t, "GET", "/assets/style.css", nil, nil)
		if resp.status != http.StatusOK {
			t.Fatalf("GET /assets/style.css: got %d, body=%s", resp.status, resp.body)
		}

		resp = unauth.do(t, "GET", "/assets/%2e%2e%2f%2e%2e%2fetc%2fpasswd", nil, nil)
		if resp.status >= 300 && resp.status < 400 {
			t.Fatalf("percent-encoded traversal must not be handled as a redirect (would bypass the guard): got %d", resp.status)
		}
		if resp.status != http.StatusBadRequest && resp.status != http.StatusNotFound {
			t.Fatalf("percent-encoded traversal: got %d (want 400 or 404), body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 12: delete with If-Match ---
	t.Run("scenario12_delete", func(t *testing.T) {
		headers := map[string]string{"Origin": "http://xordpress.test", "If-Match": `"2"`}
		resp := admin.do(t, "DELETE", fmt.Sprintf("/api/posts/%d", int64(postID)), nil, headers)
		if resp.status != http.StatusNoContent {
			t.Fatalf("delete with correct If-Match: got %d, body=%s", resp.status, resp.body)
		}

		resp = unauth.do(t, "GET", "/"+postSlug, nil, nil)
		if resp.status != http.StatusNotFound {
			t.Fatalf("GET /%s after delete: got %d (want 404), body=%s", postSlug, resp.status, resp.body)
		}

		// Wrong If-Match on the still-existing origin-test post -> 412.
		wrongHeaders := map[string]string{"Origin": "http://xordpress.test", "If-Match": `"999"`}
		resp = admin.do(t, "DELETE", fmt.Sprintf("/api/posts/%d", int64(originTestPostID)), nil, wrongHeaders)
		if resp.status != http.StatusPreconditionFailed {
			t.Fatalf("delete with wrong If-Match: got %d (want 412), body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 13: unauthenticated access + unsupported method ---
	t.Run("scenario13_unauthenticated_and_method", func(t *testing.T) {
		fresh := &apiClient{baseURL: srv.baseURL}
		resp := fresh.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "x", "slug": "x", "body": "x", "status": "draft",
		}), origin)
		if resp.status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated create: got %d (want 401), body=%s", resp.status, resp.body)
		}

		resp = fresh.do(t, "POST", "/", nil, nil)
		if resp.status != http.StatusMethodNotAllowed {
			t.Fatalf("POST /: got %d (want 405), body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 14: migration idempotency across restart ---
	var srv2 *testServer
	t.Cleanup(func() {
		// Registered on the outer *testing.T (not the scenario14 subtest's)
		// so srv2 stays up through scenario15, which runs after scenario14
		// returns.
		if srv2 != nil {
			srv2.stop()
		}
	})
	t.Run("scenario14_migration_idempotency", func(t *testing.T) {
		srv.stop()
		srv2 = startServer(t, binPath, dataDir, covDir)

		// The restarted process found users already present, so it must
		// not have generated (and printed) a new setup token.
		if srv2.setupToken != "" {
			t.Fatalf("restarted process printed a setup token despite existing users: %q", srv2.setupToken)
		}

		// design.md §8 scenario 1's "正しいトークンで再実行→401": replaying
		// the *original* token against the restarted process. No token was
		// generated this time (setupToken == ""), so the comparison always
		// fails -> 401 (design.md §4 line 180).
		fresh := &apiClient{baseURL: srv2.baseURL}
		resp := fresh.do(t, "POST", "/api/setup",
			jsonBody(t, map[string]string{"username": "someoneelse3", "password": adminPassword, "site_name": siteNameInit}),
			map[string]string{"Authorization": "Bearer " + srv.setupToken, "Content-Type": "application/json"})
		if resp.status != http.StatusUnauthorized {
			t.Fatalf("replaying original setup token after restart: got %d (want 401), body=%s", resp.status, resp.body)
		}

		// Existing data must still be reachable (migrations did not wipe
		// or fail against the already-migrated schema).
		resp = fresh.do(t, "GET", "/", nil, nil)
		if resp.status != http.StatusOK {
			t.Fatalf("GET / after restart: got %d, body=%s", resp.status, resp.body)
		}
	})

	// --- Scenario 15: update settings ---
	t.Run("scenario15_update_settings", func(t *testing.T) {
		admin2 := &apiClient{baseURL: srv2.baseURL}
		resp := admin2.login(t, adminUsername, adminPassword)
		if resp.status != http.StatusNoContent {
			t.Fatalf("login on restarted server: got %d, body=%s", resp.status, resp.body)
		}

		const newSiteName = "Renamed Site"
		resp = admin2.do(t, "PUT", "/api/settings", jsonBody(t, map[string]string{"site_name": newSiteName}),
			map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json"})
		if resp.status != http.StatusOK {
			t.Fatalf("update settings: got %d, body=%s", resp.status, resp.body)
		}

		// Publish a fresh post so there is something to observe the new
		// site_name alongside (the scenario-3/12 post was deleted above).
		resp = admin2.do(t, "POST", "/api/posts", jsonBody(t, map[string]string{
			"title": "Second Post", "slug": "second-post", "body": "<p>second</p>", "status": "published",
		}), map[string]string{"Origin": "http://xordpress.test", "Content-Type": "application/json"})
		if resp.status != http.StatusCreated {
			t.Fatalf("create second post: got %d, body=%s", resp.status, resp.body)
		}

		fresh := &apiClient{baseURL: srv2.baseURL}
		resp = fresh.do(t, "GET", "/second-post", nil, nil)
		if resp.status != http.StatusOK {
			t.Fatalf("GET /second-post: got %d, body=%s", resp.status, resp.body)
		}
		if !strings.Contains(string(resp.body), newSiteName) {
			t.Errorf("rendered HTML missing updated site_name %q: %s", newSiteName, resp.body)
		}
	})
}
