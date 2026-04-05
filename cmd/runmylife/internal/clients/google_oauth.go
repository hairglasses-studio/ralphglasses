package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"
	tasks "google.golang.org/api/tasks/v1"

	"github.com/hairglasses-studio/runmylife/internal/config"
)

// DefaultGmailScopes returns the default OAuth scopes for Gmail access.
func DefaultGmailScopes() []string {
	return []string{gmail.GmailModifyScope}
}

// DefaultCalendarScopes returns the OAuth scopes for Google Calendar read-write access.
func DefaultCalendarScopes() []string {
	return []string{calendar.CalendarScope}
}

// DefaultDriveScopes returns the OAuth scopes for Google Drive read-only access.
func DefaultDriveScopes() []string {
	return []string{drive.DriveReadonlyScope}
}

// DefaultTasksScopes returns the OAuth scopes for Google Tasks.
func DefaultTasksScopes() []string {
	return []string{tasks.TasksScope}
}

// AllGoogleScopes returns combined scopes for Gmail, Calendar, Drive, and Tasks access.
func AllGoogleScopes() []string {
	scopes := DefaultGmailScopes()
	scopes = append(scopes, DefaultCalendarScopes()...)
	scopes = append(scopes, DefaultDriveScopes()...)
	scopes = append(scopes, DefaultTasksScopes()...)
	return scopes
}

// GoogleTokenPath returns the path to the stored Google OAuth token for the default account.
func GoogleTokenPath() string {
	return GoogleTokenPathForAccount("")
}

// GoogleTokenPathForAccount returns the token path for a named account.
// Empty string or "personal" returns the default path (google_token.json).
func GoogleTokenPathForAccount(account string) string {
	if account == "" || account == "personal" {
		return filepath.Join(config.DefaultDir(), "google_token.json")
	}
	return filepath.Join(config.DefaultDir(), fmt.Sprintf("google_token_%s.json", account))
}

// GoogleCredentialsPath returns the path to the Google OAuth credentials file.
func GoogleCredentialsPath() string {
	return filepath.Join(config.DefaultDir(), "google_credentials.json")
}

// GoogleCredentialsPathForAccount returns the credentials path for a named account.
// Empty string or "personal" returns the default path (google_credentials.json).
func GoogleCredentialsPathForAccount(account string) string {
	if account == "" || account == "personal" {
		return filepath.Join(config.DefaultDir(), "google_credentials.json")
	}
	return filepath.Join(config.DefaultDir(), fmt.Sprintf("google_credentials_%s.json", account))
}

// DiscoverGoogleAccounts returns all account names that have valid token files.
// Always includes "personal" if the default token exists. Other accounts are discovered
// from files matching google_token_*.json.
func DiscoverGoogleAccounts() []string {
	var accounts []string
	if _, err := os.Stat(GoogleTokenPathForAccount("personal")); err == nil {
		accounts = append(accounts, "personal")
	}
	matches, _ := filepath.Glob(filepath.Join(config.DefaultDir(), "google_token_*.json"))
	for _, m := range matches {
		base := filepath.Base(m)
		// google_token_work.json -> work
		name := base[len("google_token_") : len(base)-len(".json")]
		if name != "" {
			accounts = append(accounts, name)
		}
	}
	return accounts
}

// LoadGoogleCredentials reads a GCP OAuth client JSON file and returns an oauth2.Config.
// Supports both "installed" (Desktop) and "web" type credentials.
// Uses AllGoogleScopes() if scopes is empty.
func LoadGoogleCredentials(credentialsPath string, scopes ...string) (*oauth2.Config, error) {
	if len(scopes) == 0 {
		scopes = AllGoogleScopes()
	}

	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}

	// Try standard parsing first (works for "installed" type with redirect_uris).
	cfg, err := google.ConfigFromJSON(data, scopes...)
	if err == nil {
		return cfg, nil
	}

	// Fall back to manual parsing for "web" type without redirect_uris.
	var raw map[string]json.RawMessage
	if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
		return nil, fmt.Errorf("parse credentials JSON: %w", jsonErr)
	}

	var creds struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		AuthURI      string `json:"auth_uri"`
		TokenURI     string `json:"token_uri"`
	}

	// Try "web" key, then "installed" key.
	for _, key := range []string{"web", "installed"} {
		if block, ok := raw[key]; ok {
			if jsonErr := json.Unmarshal(block, &creds); jsonErr == nil && creds.ClientID != "" {
				break
			}
		}
	}

	if creds.ClientID == "" {
		return nil, fmt.Errorf("parse credentials: could not find client_id in credentials file")
	}
	if creds.AuthURI == "" {
		creds.AuthURI = "https://accounts.google.com/o/oauth2/auth"
	}
	if creds.TokenURI == "" {
		creds.TokenURI = "https://oauth2.googleapis.com/token"
	}

	return &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  creds.AuthURI,
			TokenURL: creds.TokenURI,
		},
		Scopes:      scopes,
		RedirectURL: "http://localhost", // will be overridden by RunGoogleOAuthFlow
	}, nil
}

// LoadGoogleToken reads the saved OAuth token for the default account.
func LoadGoogleToken() (*oauth2.Token, error) {
	return LoadGoogleTokenForAccount("")
}

// LoadGoogleTokenForAccount reads the saved OAuth token for a named account.
func LoadGoogleTokenForAccount(account string) (*oauth2.Token, error) {
	tokenPath := GoogleTokenPathForAccount(account)
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		acctLabel := account
		if acctLabel == "" {
			acctLabel = "personal"
		}
		return nil, fmt.Errorf("read token for %s account (run 'runmylife google-auth --account %s' to authenticate): %w", acctLabel, acctLabel, err)
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	return &token, nil
}

// SaveGoogleToken writes the OAuth token for the default account.
func SaveGoogleToken(token *oauth2.Token) error {
	return SaveGoogleTokenForAccount("", token)
}

// SaveGoogleTokenForAccount writes the OAuth token for a named account.
func SaveGoogleTokenForAccount(account string, token *oauth2.Token) error {
	tokenPath := GoogleTokenPathForAccount(account)
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(tokenPath, data, 0600)
}

// RunGoogleOAuthFlow runs the desktop OAuth flow: opens a browser, waits for the callback.
func RunGoogleOAuthFlow(ctx context.Context, oauthConfig *oauth2.Config) (*oauth2.Token, error) {
	// Use a fixed port for predictable redirect URI registration.
	const callbackPort = 8976
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", callbackPort))
	if err != nil {
		return nil, fmt.Errorf("start callback listener on port %d (is it in use?): %w", callbackPort, err)
	}
	oauthConfig.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", callbackPort)

	state := fmt.Sprintf("runmylife-%d", time.Now().UnixNano())
	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("invalid state parameter")
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("oauth error: %s", errMsg)
			fmt.Fprintf(w, "<html><body><h2>Authentication failed: %s</h2><p>You can close this tab.</p></body></html>", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code received")
			http.Error(w, "No code", http.StatusBadRequest)
			return
		}
		codeCh <- code
		fmt.Fprint(w, "<html><body><h2>Authentication successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()
	defer server.Shutdown(ctx)

	// Open browser.
	fmt.Printf("\nOpening browser for Google authentication...\n")
	fmt.Printf("If it doesn't open automatically, visit:\n%s\n\n", authURL)
	openBrowser(authURL)

	// Wait for code or error.
	select {
	case code := <-codeCh:
		token, err := oauthConfig.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("exchange code: %w", err)
		}
		return token, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// savingTokenSource wraps a TokenSource and saves refreshed tokens to disk.
type savingTokenSource struct {
	base      oauth2.TokenSource
	lastToken *oauth2.Token
	account   string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	if s.lastToken == nil || tok.AccessToken != s.lastToken.AccessToken {
		_ = SaveGoogleTokenForAccount(s.account, tok)
		s.lastToken = tok
	}
	return tok, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
