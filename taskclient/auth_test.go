package taskclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCredentialRefreshCoordinatesProcessesAndPreservesPrivateFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		r.ParseForm()
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Error("refresh credential replayed")
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 900, "token_type": "Bearer"})
	}))
	defer server.Close()
	path, err := CredentialPath(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials(path, Credentials{Origin: server.URL, ClientID: "client", AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := CredentialSource(server.URL, path)(context.Background())
			if err != nil || token != "new-access" {
				t.Errorf("token source failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("refresh requests: %d", calls.Load())
	}
	c, err := LoadCredentials(path)
	if err != nil || c.RefreshToken != "new-refresh" || c.RefreshUncertain {
		t.Fatal("rotated credentials not durable")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("credentials not private")
	}
	info, _ = os.Stat(filepath.Dir(path))
	if info.Mode().Perm() != 0700 {
		t.Fatal("directory not private")
	}
}

func TestCredentialRefreshUnsentWhileOfflineKeepsRefreshTokenUsable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	listener.Close() // offline, as after waking from sleep
	path, err := CredentialPath(origin)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials(path, Credentials{Origin: origin, ClientID: "client", AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := CredentialSource(origin, path)(context.Background()); !errors.Is(err, errAuthNotSent) {
		t.Fatalf("offline refresh error: %v", err)
	}
	c, err := LoadCredentials(path)
	if err != nil || c.RefreshUncertain || c.RefreshToken != "old-refresh" {
		t.Fatal("unsent refresh marked the refresh token uncertain")
	}
	listener, err = net.Listen("tcp4", listener.Addr().String())
	if err != nil {
		t.Skip("loopback port reused:", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Error("unexpected refresh token")
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 900, "token_type": "Bearer"})
	})}
	go server.Serve(listener)
	defer server.Close()
	if token, err := CredentialSource(origin, path)(context.Background()); err != nil || token != "new-access" {
		t.Fatalf("refresh after reconnect: %q %v", token, err)
	}
}

func TestCredentialRefreshNeverUsesPooledConnection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	type served struct{ requests atomic.Int32 }
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(served{}).(*served).requests.Add(1) > 1 {
			// A connection that died during sleep: the request is lost unanswered.
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		if r.URL.Path == "/oauth/token" {
			json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 900, "token_type": "Bearer"})
		}
	}))
	server.Config.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
		return context.WithValue(ctx, served{}, &served{})
	}
	server.Start()
	defer server.Close()
	// Task polling leaves an idle keep-alive connection in the default pool.
	r, err := http.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	path, err := CredentialPath(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials(path, Credentials{Origin: server.URL, ClientID: "client", AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if token, err := CredentialSource(server.URL, path)(context.Background()); err != nil || token != "new-access" {
		t.Fatalf("refresh after sleep: %q %v", token, err)
	}
}

func TestDesktopLoginBindsStateAndPKCEToRegisteredLoopback(t *testing.T) {
	var redirect, challenge string
	var exchanged atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/register":
			var input struct {
				Redirects []string `json:"redirect_uris"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Redirects) != 1 {
				t.Error("missing loopback registration")
				http.Error(w, "invalid", 400)
				return
			}
			redirect = input.Redirects[0]
			json.NewEncoder(w).Encode(map[string]string{"client_id": "desktop"})
		case "/oauth/token":
			r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("client_id") != "desktop" || r.Form.Get("code") != "approved" || r.Form.Get("redirect_uri") != redirect || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
				t.Error("token exchange did not bind original PKCE/redirect")
			}
			exchanged.Store(true)
			json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "expires_in": 900, "token_type": "Bearer"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	credentials, err := Login(ctx, server.URL+"/", io.Discard, func(location string) error {
		u, err := url.Parse(location)
		if err != nil {
			return err
		}
		q := u.Query()
		challenge = q.Get("code_challenge")
		if q.Get("resource") != server.URL || q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != redirect {
			t.Error("incorrect authorization request")
		}
		for _, state := range []string{"wrong-state", q.Get("state")} {
			callback := redirect + "?" + url.Values{"state": {state}, "code": {"approved"}}.Encode()
			r, err := http.Get(callback)
			if err != nil {
				return err
			}
			r.Body.Close()
			if state == "wrong-state" && r.StatusCode != http.StatusBadRequest {
				t.Error("wrong state accepted")
			}
		}
		return nil
	})
	if err != nil || !exchanged.Load() || credentials.Origin != server.URL || credentials.RefreshToken != "refresh" {
		t.Fatalf("desktop login failed: %v", err)
	}
}
func TestLostRefreshResponseRequiresLoginInsteadOfReplaying(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "lost", 502) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.json")
	SaveCredentials(path, Credentials{Origin: server.URL, ClientID: "client", AccessToken: "old", RefreshToken: "refresh", ExpiresAt: time.Now()})
	source := CredentialSource(server.URL, path)
	if _, err := source(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}
	if _, err := source(context.Background()); err == nil {
		t.Fatal("uncertain credential reused")
	}
	if calls.Load() != 1 {
		t.Fatal("one-time credential was sent twice")
	}
}
func TestCredentialSourceRejectsWrongOriginAndPublicPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	SaveCredentials(path, Credentials{Origin: "https://one.example", ClientID: "client", AccessToken: "token", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)})
	if _, err := CredentialSource("https://two.example", path)(context.Background()); err == nil {
		t.Fatal("wrong origin accepted")
	}
	os.Chmod(path, 0644)
	if _, err := LoadCredentials(path); err == nil {
		t.Fatal("public credentials accepted")
	}
}
