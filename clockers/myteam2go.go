package clockers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// MyTeam2GoClocker implements the Clocker interface for MyTeam2Go system.
type MyTeam2GoClocker struct {
	baseURL   string
	username  string
	password  string
	latitude  float64
	longitude float64
	client    *http.Client
}

// NewMyTeam2GoClocker creates a new MyTeam2Go clocker instance.
func NewMyTeam2GoClocker(company, username, password string, latitude, longitude float64) *MyTeam2GoClocker {
	jar, _ := cookiejar.New(nil)
	return &MyTeam2GoClocker{
		baseURL:   "https://" + company + ".myteam2go.com",
		username:  username,
		password:  password,
		latitude:  latitude,
		longitude: longitude,
		client: &http.Client{
			Jar: jar,
		},
	}
}

// ClockIn sends a clock-in request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockIn(ctx context.Context) error {
	return nil
}

// ClockOut sends a clock-out request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockOut(ctx context.Context) error {
	return nil
}

// ClockPause sends a pause request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockPause(ctx context.Context) error {
	return nil
}

// ClockResume sends a resume request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockResume(ctx context.Context) error {
	return nil
}

// login authenticates the user by sending a POST request to the login endpoint with username and password credentials.
// It retrieves and stores the JSESSIONID cookie for session management. Returns an error if login fails.
func (c *MyTeam2GoClocker) login(ctx context.Context) error {
	loginURL := c.baseURL + "/j_security_check"

	credentials := url.Values{}
	credentials.Set("username", c.username)
	credentials.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(credentials.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	slog.Debug("Attempting login to MyTeam2Go", "url", loginURL, "username", c.username)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if strings.Contains(resp.Request.URL.String(), "error=true") {
		return fmt.Errorf("invalid credentials or login failed")
	}

	var token string
	u, err := url.Parse(c.baseURL)
	if err == nil {
		for _, cookie := range c.client.Jar.Cookies(u) {
			if cookie.Name == "JSESSIONID" {
				token = cookie.Value
				break
			}
		}
	}

	if token == "" {
		return fmt.Errorf("login failed: JSESSIONID cookie not found")
	}

	slog.Debug("Login successful", "JSESSIONID", token)
	return nil
}
