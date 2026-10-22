// service-smoke exercises the Rust service through the future Go TUI's HTTP
// client. It only targets the local Worker and never reads task/config files.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/dtt101/doitdoit/taskclient"
)

const localOrigin = "http://localhost:8787"

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type smokeClient struct {
	origin string
	http   *http.Client
	out    io.Writer
}

func main() {
	origin := flag.String("url", localOrigin, "Service origin; production requires an explicit HTTPS URL")
	extended := flag.Bool("extended", false, "Test desktop refresh, new operations and export using disposable tasks")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	fmt.Println("Testing", *origin, "using clearly labelled disposable tasks.")
	var err error
	if *extended {
		err = runExtended(ctx, *origin, os.Stdout, openBrowser)
	} else {
		err = run(ctx, *origin, os.Stdout, openBrowser)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Smoke test failed:", err)
		os.Exit(1)
	}
}

func openBrowser(location string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, location)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func randomString() string {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	return base64.RawURLEncoding.EncodeToString(data)
}
func operationID() string {
	data := make([]byte, 16)
	_, _ = rand.Read(data)
	data[6] = data[6]&0x0f | 0x40
	data[8] = data[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", data[:4], data[4:6], data[6:8], data[8:10], data[10:])
}

func (c *smokeClient) request(ctx context.Context, path, contentType string, input []byte, token string, output any) error {
	method := http.MethodPost
	if input == nil {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, bytes.NewReader(input))
	if err != nil {
		return errors.New("could not build local service request")
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if path == "/mcp" {
		req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("could not reach %s; keep make dev running", c.origin)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return errors.New("incomplete or oversized service response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never echo response bodies, OAuth codes, or tokens in terminal errors.
		return fmt.Errorf("%s returned HTTP %d; check local migrations and .dev.vars, then restart make dev", path, resp.StatusCode)
	}
	if output != nil && json.Unmarshal(data, output) != nil {
		return fmt.Errorf("%s returned invalid JSON", path)
	}
	return nil
}
func (c *smokeClient) json(ctx context.Context, path string, input any, token string, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return c.request(ctx, path, "application/json", data, token, output)
}

// login binds a random callback port only on loopback. State and PKCE secrets
// stay in memory, and mismatched callbacks cannot consume a legitimate login.
func (c *smokeClient) login(ctx context.Context, open func(string) error) (tokenResponse, string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return tokenResponse{}, "", errors.New("could not listen for the local login callback")
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	verifier, state := randomString(), randomString()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	var registered struct {
		ID string `json:"client_id"`
	}
	if err = c.json(ctx, "/oauth/register", map[string]any{
		"client_name": "doitdoit local smoke test", "redirect_uris": []string{redirect},
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"},
	}, "", &registered); err != nil {
		return tokenResponse{}, "", err
	}
	if registered.ID == "" {
		return tokenResponse{}, "", errors.New("registration returned no client ID")
	}
	type outcome struct {
		code string
		err  error
	}
	result := make(chan outcome, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			http.Error(w, "Expected GET", 405)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query["state"]) != 1 || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid login state. Return to the login you started.", 400)
			return
		}
		var value outcome
		if query.Get("error") != "" {
			value.err = errors.New("login was denied; run make smoke again to retry")
		} else if len(query["code"]) != 1 || query.Get("code") == "" {
			http.Error(w, "Missing authorization code", 400)
			return
		} else {
			value.code = query.Get("code")
		}
		select {
		case result <- value:
			fmt.Fprintln(w, "Login received. Return to your terminal for the test results.")
		default:
			http.Error(w, "Login already received", 409)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	authURL := c.origin + "/authorize?" + url.Values{
		"response_type": {"code"}, "client_id": {registered.ID}, "redirect_uri": {redirect},
		"scope": {"tasks:read tasks:write"}, "state": {state}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "resource": {c.origin},
	}.Encode()
	fmt.Fprintln(c.out, "Opening login. Approve ‘doitdoit local smoke test’, then sign in with GitHub.")
	fmt.Fprintln(c.out, "If the browser does not open, visit:\n"+authURL)
	if err = open(authURL); err != nil {
		fmt.Fprintln(c.out, "Could not open the browser automatically; use the link above.")
	}
	var code string
	select {
	case <-ctx.Done():
		return tokenResponse{}, "", ctx.Err()
	case value := <-result:
		if value.err != nil {
			return tokenResponse{}, "", value.err
		}
		code = value.code
	}
	data := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {registered.ID},
		"redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {c.origin}}.Encode()
	var token tokenResponse
	err = c.request(ctx, "/oauth/token", "application/x-www-form-urlencoded", []byte(data), "", &token)
	if err == nil && (token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer")) {
		err = errors.New("login returned no usable access token")
	}
	return token, registered.ID, err
}

func run(ctx context.Context, origin string, out io.Writer, open func(string) error) (err error) {
	c := &smokeClient{origin: origin, out: out, http: &http.Client{Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var health struct {
		Status string `json:"status"`
	}
	if err = c.request(ctx, "/health", "", nil, "", &health); err != nil {
		return err
	}
	if health.Status != "ok" {
		return errors.New("local service is not healthy")
	}
	token, clientID, err := c.login(ctx, open)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "PASS: GitHub login and token exchange")
	defer func() {
		// Revoke even after a failed test or Ctrl-C, without persisting credentials.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data := url.Values{"token": {token.AccessToken}, "client_id": {clientID}}.Encode()
		if revokeErr := c.request(cleanup, "/oauth/revoke", "application/x-www-form-urlencoded", []byte(data), "", nil); revokeErr != nil {
			if err == nil {
				err = errors.New("test finished but temporary token revocation failed")
			}
			fmt.Fprintln(out, "Temporary token revocation failed; the token will expire automatically.")
		} else {
			fmt.Fprintln(out, "Temporary login revoked; no credentials saved.")
		}
	}()
	client, err := taskclient.New(origin, c.http, func(context.Context) (string, error) { return token.AccessToken, nil })
	if err != nil {
		return err
	}
	past := "2020-02-29"
	task, err := client.Create(ctx, taskclient.CreateRequest{OperationID: operationID(), Title: "[local smoke test] doitdoit service", Notes: "Created by make smoke. Safe to remove.", ScheduledDate: &past})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Created test task:", task.ID)
	if task.ScheduledDate == nil || *task.ScheduledDate != past {
		return errors.New("create did not preserve the supplied past date")
	}
	future := "2030-01-02"
	datePointer := &future
	update := taskclient.UpdateRequest{OperationID: operationID(), ExpectedRevision: task.Revision, Changes: taskclient.Changes{ScheduledDate: &datePointer}}
	updated, err := client.Update(ctx, task.ID, update)
	if err != nil {
		return err
	}
	if updated.ScheduledDate == nil || *updated.ScheduledDate != future || updated.Revision != task.Revision+1 {
		return errors.New("date update returned an unexpected result")
	}
	retried, err := client.Update(ctx, task.ID, update)
	if err != nil {
		return err
	}
	if retried.Revision != updated.Revision || retried.ScheduledDate == nil || *retried.ScheduledDate != future {
		return errors.New("identical retry changed the task")
	}
	pastPointer := &past
	stale := taskclient.UpdateRequest{OperationID: operationID(), ExpectedRevision: task.Revision, Changes: taskclient.Changes{ScheduledDate: &pastPointer}}
	_, err = client.Update(ctx, task.ID, stale)
	var conflict *taskclient.APIError
	if !errors.As(err, &conflict) || conflict.Status != 409 || conflict.Code != "revision_conflict" || conflict.Current == nil || conflict.Current.ScheduledDate == nil || *conflict.Current.ScheduledDate != future {
		return errors.New("stale write did not return the current date as a conflict")
	}
	fmt.Fprintln(out, "PASS: date update, identical retry, and stale-write conflict")
	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err = c.json(ctx, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "doitdoit local smoke test", "version": "1"}}}, token.AccessToken, &rpc); err != nil {
		return err
	}
	var initialized struct {
		Protocol string `json:"protocolVersion"`
	}
	if json.Unmarshal(rpc.Result, &initialized) != nil || initialized.Protocol != "2025-11-25" {
		return errors.New("MCP initialization failed")
	}
	rpc.Result = nil
	if err = c.json(ctx, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "get_task", "arguments": map[string]string{"id": task.ID}}}, token.AccessToken, &rpc); err != nil {
		return err
	}
	var read struct {
		IsError    bool `json:"isError"`
		Structured struct {
			Task taskclient.Task `json:"task"`
		} `json:"structuredContent"`
	}
	if json.Unmarshal(rpc.Result, &read) != nil || read.IsError || read.Structured.Task.ID != task.ID || read.Structured.Task.ScheduledDate == nil || *read.Structured.Task.ScheduledDate != future || read.Structured.Task.Revision != updated.Revision {
		return errors.New("MCP did not read back the updated task")
	}
	fmt.Fprintln(out, "PASS: MCP initialization and task read-back")
	completed := "completed"
	finished, err := client.Update(ctx, task.ID, taskclient.UpdateRequest{OperationID: operationID(), ExpectedRevision: updated.Revision, Changes: taskclient.Changes{Status: &completed}})
	if err != nil {
		return err
	}
	if finished.Status != completed || finished.ScheduledDate == nil || *finished.ScheduledDate != future {
		return errors.New("completion did not preserve the scheduled date")
	}
	fmt.Fprintln(out, "PASS: test task marked completed. Local smoke test passed.")
	return nil
}
