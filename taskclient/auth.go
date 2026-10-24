package taskclient

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Credentials struct {
	Origin           string    `json:"origin"`
	ClientID         string    `json:"client_id"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshUncertain bool      `json:"refresh_uncertain,omitempty"`
}

func CredentialPath(origin string) (string, error) {
	if _, err := New(origin, nil, func(context.Context) (string, error) { return "", nil }); err != nil {
		return "", err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.TrimSuffix(origin, "/")))
	return filepath.Join(dir, "doitdoit", "services", hex.EncodeToString(sum[:16]), "credentials.json"), nil
}

// WritePrivate atomically replaces a private file, syncing bytes before rename.
func WritePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".doitdoit-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func SaveCredentials(path string, c Credentials) error {
	c.Origin = strings.TrimSuffix(c.Origin, "/")
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return WritePrivate(path, data)
}
func LoadCredentials(path string) (Credentials, error) {
	var c Credentials
	info, err := os.Lstat(path)
	if err != nil {
		return c, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return c, errors.New("credentials must be a private regular file (0600)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if len(data) > 32768 || json.Unmarshal(data, &c) != nil || c.ClientID == "" || c.AccessToken == "" {
		return c, errors.New("invalid credentials; sign in again")
	}
	return c, nil
}
func authRequest(ctx context.Context, origin, path string, input any, form bool, output any) error {
	var body string
	content := "application/json"
	if form {
		body = input.(url.Values).Encode()
		content = "application/x-www-form-urlencoded"
	} else {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = string(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+path, strings.NewReader(body))
	if err != nil {
		return errors.New("invalid OAuth request")
	}
	req.Header.Set("Content-Type", content)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := client.Do(req)
	if err != nil {
		return errors.New("OAuth response unavailable; sign in again if refresh outcome is uncertain")
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(data) > 65536 {
		return errors.New("invalid OAuth response")
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("OAuth request rejected (HTTP %d); sign in again", r.StatusCode)
	}
	if output != nil && json.Unmarshal(data, output) != nil {
		return errors.New("invalid OAuth JSON")
	}
	return nil
}

type authTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (t authTokens) credentials(origin, client string) (Credentials, error) {
	if t.AccessToken == "" || t.RefreshToken == "" || !strings.EqualFold(t.TokenType, "Bearer") || t.ExpiresIn < 60 || t.ExpiresIn > 86400 {
		return Credentials{}, errors.New("invalid token response")
	}
	return Credentials{Origin: origin, ClientID: client, AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)}, nil
}

// CredentialSource locks the credential store across processes before refreshing.
// A lost rotation response is never retried with a possibly spent refresh token.
func CredentialSource(origin, path string) TokenSource {
	origin = strings.TrimSuffix(origin, "/")
	return func(ctx context.Context) (string, error) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return "", err
		}
		defer lock.Close()
		for {
			err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if err == nil {
				break
			}
			if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
				return "", err
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
		defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		c, err := LoadCredentials(path)
		if err != nil {
			return "", err
		}
		if c.Origin != origin || c.RefreshUncertain {
			return "", errors.New("sign in again for this service")
		}
		if time.Until(c.ExpiresAt) > time.Minute {
			return c.AccessToken, nil
		}
		// Persist uncertainty before sending the one-time refresh credential. A crash
		// before the replacement is durable must require login instead of token replay.
		c.RefreshUncertain = true
		if err := SaveCredentials(path, c); err != nil {
			return "", err
		}
		var tokens authTokens
		err = authRequest(ctx, origin, "/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {c.ClientID}, "refresh_token": {c.RefreshToken}, "resource": {origin}}, true, &tokens)
		if err != nil {
			return "", err
		}
		next, err := tokens.credentials(origin, c.ClientID)
		if err != nil {
			return "", err
		}
		if err = SaveCredentials(path, next); err != nil {
			return "", err
		}
		return next.AccessToken, nil
	}
}

// Login uses a private browser-bound PKCE flow; no password or upstream token is handled here.
func Login(ctx context.Context, origin string, out io.Writer, openBrowser func(string) error) (Credentials, error) {
	origin = strings.TrimSuffix(origin, "/")
	if _, err := New(origin, nil, func(context.Context) (string, error) { return "", nil }); err != nil {
		return Credentials{}, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return Credentials{}, err
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	verifier := OperationID() + OperationID()
	state := OperationID() + OperationID()
	sum := sha256.Sum256([]byte(verifier))
	var registration struct {
		ClientID string `json:"client_id"`
	}
	if err := authRequest(ctx, origin, "/oauth/register", map[string]any{"client_name": "doitdoit terminal", "redirect_uris": []string{redirect}, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}}, false, &registration); err != nil {
		return Credentials{}, err
	}
	if registration.ClientID == "" {
		return Credentials{}, errors.New("registration returned no client")
	}
	result := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || r.Method != "GET" || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid login state", 400)
			return
		}
		if q.Get("error") != "" {
			select {
			case result <- "":
			default:
			}
			http.Error(w, "Login denied", 400)
			return
		}
		if len(q["code"]) != 1 || q.Get("code") == "" {
			http.Error(w, "Missing login code", 400)
			return
		}
		select {
		case result <- q.Get("code"):
		default:
		}
		fmt.Fprintln(w, "Signed in. Return to the terminal.")
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	params := url.Values{"response_type": {"code"}, "client_id": {registration.ClientID}, "redirect_uri": {redirect}, "resource": {origin}, "scope": {"tasks:read tasks:write offline_access"}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}}
	location := origin + "/authorize?" + params.Encode()
	fmt.Fprintln(out, "Approve ‘doitdoit terminal’ in the browser, then sign in with GitHub.")
	if err := openBrowser(location); err != nil {
		fmt.Fprintln(out, "Open:", location)
	}
	select {
	case <-ctx.Done():
		return Credentials{}, ctx.Err()
	case code := <-result:
		if code == "" {
			return Credentials{}, errors.New("login denied")
		}
		var tokens authTokens
		if err := authRequest(ctx, origin, "/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {registration.ClientID}, "code": {code}, "redirect_uri": {redirect}, "resource": {origin}, "code_verifier": {verifier}}, true, &tokens); err != nil {
			return Credentials{}, err
		}
		return tokens.credentials(origin, registration.ClientID)
	}
}
func Logout(ctx context.Context, c Credentials) error {
	c.Origin = strings.TrimSuffix(c.Origin, "/")
	if _, err := New(c.Origin, nil, func(context.Context) (string, error) { return "", nil }); err != nil {
		return err
	}
	return authRequest(ctx, c.Origin, "/oauth/revoke", url.Values{"client_id": {c.ClientID}, "token": {c.RefreshToken}}, true, nil)
}

// WritePrivateExclusive publishes a complete private export without replacing an existing file.
func WritePrivateExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".doitdoit-export-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Link(temp, path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
