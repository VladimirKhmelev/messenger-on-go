package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const (
	// an unresponsive GitHub must not pin the login request forever
	githubRequestTimeout = 10 * time.Second

	githubTokenURL  = "https://github.com/login/oauth/access_token"
	githubUserURL   = "https://api.github.com/user"
	githubEmailsURL = "https://api.github.com/user/emails"
)

type GitHubClient struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewGitHubClient(clientID, clientSecret string) *GitHubClient {
	return &GitHubClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: githubRequestTimeout},
	}
}

func (c *GitHubClient) FetchProfile(ctx context.Context, code string) (*domain.GitHubProfile, error) {
	token, err := c.exchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}

	profile, err := c.fetchUser(ctx, token)
	if err != nil {
		return nil, err
	}

	if profile.Email == "" {
		email, err := c.fetchPrimaryVerifiedEmail(ctx, token)
		if err != nil {
			return nil, err
		}
		profile.Email = email
	}

	return profile, nil
}

func (c *GitHubClient) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.URL.RawQuery = form.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Error == "bad_verification_code" {
		// expired, already used, or a code someone else pasted in: the
		// client's fault, not ours
		return "", domain.ErrInvalidOAuthCode
	}
	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s: %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", errors.New("github oauth: empty access token")
	}

	return result.AccessToken, nil
}

func (c *GitHubClient) fetchUser(ctx context.Context, token string) (*domain.GitHubProfile, error) {
	var raw struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
	}
	if err := c.getJSON(ctx, githubUserURL, token, &raw); err != nil {
		return nil, err
	}

	return &domain.GitHubProfile{ID: raw.ID, Login: raw.Login, Email: raw.Email}, nil
}

func (c *GitHubClient) fetchPrimaryVerifiedEmail(ctx context.Context, token string) (string, error) {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := c.getJSON(ctx, githubEmailsURL, token, &emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}

	return "", domain.ErrOAuthNoVerifiedEmail
}

func (c *GitHubClient) getJSON(ctx context.Context, url, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api %s: status %d: %s", url, resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
