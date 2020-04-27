package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/reddit/baseplate.go/log"
	"github.com/reddit/baseplate.go/randbp"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

// OAuthClientConfig defines the configurations needed for the oauth client.
type OAuthClientConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

// Args passed to GetOAuthClient function.
type Args struct {
	Directory string
	Profile   string
	NoAuth    bool
}

// GetOAuthClient returns an HTTP client that's ready to be used with Drive API.
func GetOAuthClient(
	ctx context.Context,
	args Args,
	cfg OAuthClientConfig,
) *http.Client {
	config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  `https://accounts.google.com/o/oauth2/auth`,
			TokenURL: `https://accounts.google.com/o/oauth2/token`,
		},
		RedirectURL: `urn:ietf:wg:oauth:2.0:oob`,
		Scopes: []string{
			`https://www.googleapis.com/auth/drive`,
		},
	}

	tokFile := filepath.Join(args.Directory, args.Profile+".json")
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		if args.NoAuth {
			log.Fatalw("Unable to authenticate", "profile", args.Profile, "err", err)
		}
		tok = getTokenFromWeb(ctx, config)
		saveToken(tokFile, tok)
	}
	return config.Client(ctx, tok)
}

func getTokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL(fmt.Sprintf("state-%d", randbp.R.Uint64()))
	fmt.Printf(
		"Go to the following link in your browser then type the authorization code: \n%s\n",
		authURL,
	)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalw("Unable to read authorization code", "err", err)
	}

	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		log.Fatalw("Unable to retrieve token from web", "err", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalw("Unable to cache oauth token", "path", path, "err", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Fatalw("Unable to save oauth token", "path", path, "err", err)
		}
	}()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		log.Fatalw("Unable to encode profile token json", "err", err)
	}
}
