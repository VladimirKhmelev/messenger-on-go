package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/domain"
)

const (
	githubTokenURL  = "https://github.com/login/oauth/access_token"
	githubUserURL   = "https://api.github.com/user"
	githubEmailsURL = "https://api.github.com/user/emails"
)

type GitHubProfile struct {
	ID    int64
	Login string
	Email string
}

type GitHubClient struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewGitHubClient(clientID, clientSecret string) *GitHubClient {
	return &GitHubClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{},
	}
}

func (c *GitHubClient) FetchProfile(code string) (*GitHubProfile, error) {
	token, err := c.exchangeCode(code)
	if err != nil {
		return nil, err
	}

	profile, err := c.fetchUser(token)
	if err != nil {
		return nil, err
	}

	if profile.Email == "" {
		email, err := c.fetchPrimaryVerifiedEmail(token)
		if err != nil {
			return nil, err
		}
		profile.Email = email
	}

	return profile, nil
}

func (c *GitHubClient) exchangeCode(code string) (string, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
	}

	req, err := http.NewRequest(http.MethodPost, githubTokenURL, nil)
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

	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s: %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", errors.New("github oauth: empty access token")
	}

	return result.AccessToken, nil
}

func (c *GitHubClient) fetchUser(token string) (*GitHubProfile, error) {
	var raw struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
	}
	if err := c.getJSON(githubUserURL, token, &raw); err != nil {
		return nil, err
	}

	return &GitHubProfile{ID: raw.ID, Login: raw.Login, Email: raw.Email}, nil
}

func (c *GitHubClient) fetchPrimaryVerifiedEmail(token string) (string, error) {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := c.getJSON(githubEmailsURL, token, &emails); err != nil {
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

func (c *GitHubClient) getJSON(url, token string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
