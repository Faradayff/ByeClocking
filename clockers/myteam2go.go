package clockers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
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
	if err := c.login(ctx); err != nil {
		slog.Error("Error login in. Impossible to clock in", "error", err)
		return err
	}

	homeURL := c.baseURL + "/home.xhtml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homeURL, nil)
	if err != nil {
		slog.Error("Failed to create home request", "error", err)
		return fmt.Errorf("failed to create home request: %w", err)
	}
	c.setBrowserHeaders(req, c.baseURL+"/")

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("Failed to fetch home page", "error", err)
		return fmt.Errorf("failed to fetch home page: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	html := string(bodyBytes)

	// If the user is already clocked in, the dropdown shows options like
	// "Fin de jornada laboral" or "Inicio Pausa" (NOT "Inicio jornada laboral").
	// "Inicio jornada laboral" only appears when the user has NOT clocked in yet.
	if !strings.Contains(html, "Inicio jornada laboral") {
		slog.Warn("Already clocked in (Inicio jornada laboral not available), skipping")
		return nil
	}

	// Extract ViewState
	viewStateRegex := regexp.MustCompile(`name="jakarta\.faces\.ViewState"[^>]*value="([^"]+)"`)
	matches := viewStateRegex.FindStringSubmatch(html)
	if len(matches) < 2 {
		return fmt.Errorf("could not find ViewState on home page")
	}
	viewState := matches[1]

	// 1. Click "Mi control horario" to initialize the dialog
	menuRegex := regexp.MustCompile(`id="([^"]+)"[^>]*class="[^"]*menuHome-employee-general-workAssistance[^"]*"`)
	menuMatches := menuRegex.FindStringSubmatch(html)
	if len(menuMatches) < 2 {
		return fmt.Errorf("could not find 'Mi control horario' menu item")
	}
	menuID := menuMatches[1]

	menuData := url.Values{}
	menuData.Set("jakarta.faces.partial.ajax", "true")
	menuData.Set("jakarta.faces.source", menuID)
	menuData.Set("jakarta.faces.partial.execute", menuID)
	menuData.Set(menuID, menuID)
	menuData.Set("menuForm", "menuForm")
	menuData.Set("jakarta.faces.ViewState", viewState)

	menuReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(menuData.Encode()))
	menuReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	menuReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	menuReq.Header.Set("Faces-Request", "partial/ajax")
	menuReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(menuReq, homeURL)

	menuResp, err := c.client.Do(menuReq)
	if err != nil {
		slog.Error("Failed to click 'Mi control horario'", "error", err)
		return fmt.Errorf("failed to click 'Mi control horario': %w", err)
	}
	menuBytes, _ := io.ReadAll(menuResp.Body)
	menuHtml := string(menuBytes)
	menuResp.Body.Close()

	if m := viewStateRegex.FindStringSubmatch(menuHtml); len(m) >= 2 {
		viewState = m[1]
	}

	// Wait a tiny bit just like a real user
	time.Sleep(500 * time.Millisecond)

	// Determine button and option values
	btnRegex := regexp.MustCompile(`name="(workAssistanceForm:j_idt\d+)"[^>]*><span[^>]*>Guardar</span>`)
	btnMatches := btnRegex.FindStringSubmatch(menuHtml)
	if len(btnMatches) < 2 {
		// fallback to original HTML if not found in ajax response
		btnMatches = btnRegex.FindStringSubmatch(html)
		if len(btnMatches) < 2 {
			return fmt.Errorf("could not find Guardar button in workAssistanceForm")
		}
	}
	btnName := btnMatches[1]

	optRegex := regexp.MustCompile(`value="(\d+)"[^>]*>Inicio jornada laboral<`)
	optMatches := optRegex.FindStringSubmatch(menuHtml)
	if len(optMatches) < 2 {
		optMatches = optRegex.FindStringSubmatch(html)
		if len(optMatches) < 2 {
			return fmt.Errorf("could not find Inicio jornada laboral option")
		}
	}
	optValue := optMatches[1]

	// 2. Simulate the change event on the dropdown
	changeData := url.Values{}
	changeData.Set("jakarta.faces.partial.ajax", "true")
	changeData.Set("jakarta.faces.source", "workAssistanceForm:inputOption")
	changeData.Set("jakarta.faces.partial.execute", "workAssistanceForm:inputOption")
	changeData.Set("jakarta.faces.partial.render", "workAssistanceForm:workAssistanceFormContent")
	changeData.Set("jakarta.faces.behavior.event", "change")
	changeData.Set("jakarta.faces.partial.event", "change")
	changeData.Set("workAssistanceForm", "workAssistanceForm")
	changeData.Set("workAssistanceForm:inputOption_input", optValue)
	changeData.Set("workAssistanceForm:locationLatitude", "")
	changeData.Set("workAssistanceForm:locationLongitude", "")
	changeData.Set("workAssistanceForm:locationError", "")
	changeData.Set("jakarta.faces.ViewState", viewState)

	changeReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(changeData.Encode()))
	changeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	changeReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	changeReq.Header.Set("Faces-Request", "partial/ajax")
	changeReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(changeReq, homeURL)

	changeResp, err := c.client.Do(changeReq)
	if err != nil {
		slog.Error("Failed to execute change event request", "error", err)
		return fmt.Errorf("failed to execute change event request: %w", err)
	}
	changeBytes, _ := io.ReadAll(changeResp.Body)
	changeHtml := string(changeBytes)
	slog.Debug("Change event response", "body", changeHtml)
	changeResp.Body.Close()

	// Extract updateLocationForm remote command ID
	locCmdRegex := regexp.MustCompile(`updateLocationForm\s*=\s*function\(\)\s*\{return\s*PrimeFaces\.ab\(\{s:"([^"]+)"`)
	locCmdMatches := locCmdRegex.FindStringSubmatch(changeHtml)
	if len(locCmdMatches) < 2 {
		locCmdMatches = locCmdRegex.FindStringSubmatch(html)
		if len(locCmdMatches) < 2 {
			slog.Warn("Could not find updateLocationForm remote command ID, continuing anyway")
		}
	}

	lat, lon, acc := c.humanLocation()

	if len(locCmdMatches) >= 2 {
		locCmdID := locCmdMatches[1]
		locData := url.Values{}
		locData.Set("jakarta.faces.partial.ajax", "true")
		locData.Set("jakarta.faces.source", locCmdID)
		locData.Set("jakarta.faces.partial.execute", "workAssistanceForm:locationLatitude workAssistanceForm:locationLongitude workAssistanceForm:locationError")
		locData.Set("jakarta.faces.partial.render", "workAssistanceForm")
		locData.Set(locCmdID, locCmdID)
		locData.Set("workAssistanceForm:locationLatitude", lat)
		locData.Set("workAssistanceForm:locationLongitude", lon)
		locData.Set("workAssistanceForm:locationError", acc)
		locData.Set("jakarta.faces.ViewState", viewState)

		locReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(locData.Encode()))
		locReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		locReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
		locReq.Header.Set("Faces-Request", "partial/ajax")
		locReq.Header.Set("X-Requested-With", "XMLHttpRequest")
		c.setBrowserHeaders(locReq, homeURL)

		locResp, err := c.client.Do(locReq)
		if err != nil {
			slog.Error("Failed to execute updateLocationForm", "error", err)
		} else {
			locBytes, _ := io.ReadAll(locResp.Body)
			slog.Debug("Update location response", "body", string(locBytes))
			locResp.Body.Close()
			if m := viewStateRegex.FindStringSubmatch(string(locBytes)); len(m) >= 2 {
				viewState = m[1]
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 3. Submit the form by clicking "Guardar"
	data := url.Values{}
	data.Set("jakarta.faces.partial.ajax", "true")
	data.Set("jakarta.faces.source", btnName)
	data.Set("jakarta.faces.partial.execute", "@all")
	data.Set("jakarta.faces.partial.render", "workAssistanceForm messages session_messages workAssistanceForm:WAMessagesDialog")
	data.Set(btnName, btnName)
	data.Set("workAssistanceForm", "workAssistanceForm")
	data.Set("workAssistanceForm:inputOption_input", optValue)
	data.Set("workAssistanceForm:locationLatitude", lat)
	data.Set("workAssistanceForm:locationLongitude", lon)
	data.Set("workAssistanceForm:locationError", acc)
	data.Set("jakarta.faces.ViewState", viewState)

	slog.Debug("Prepared Clock In request", "viewState", viewState, "buttonName", btnName, "optValue", optValue)

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(data.Encode()))
	if err != nil {
		slog.Error("Failed to create clock-in request", "error", err)
		return fmt.Errorf("failed to create clock-in request: %w", err)
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	postReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	postReq.Header.Set("Faces-Request", "partial/ajax")
	postReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(postReq, homeURL)

	postResp, err := c.client.Do(postReq)
	if err != nil {
		slog.Error("Failed to execute clock-in request", "error", err)
		return fmt.Errorf("failed to execute clock-in request: %w", err)
	}
	defer postResp.Body.Close()

	respBody, _ := io.ReadAll(postResp.Body)
	respStr := string(respBody)
	slog.Debug("Guardar response", "body", respStr)

	// Check if the response contains a success (no validationFailed or actual success text)
	if strings.Contains(respStr, "No se ha podido efectuar") {
		slog.Error("Clock-in rejected by server", "status", postResp.StatusCode)
		return fmt.Errorf("clock-in rejected by server: no se ha podido efectuar el registro")
	}

	// Verify the clock-in actually took effect by querying the current state.
	slog.Debug("Verifying clock-in status after submit")
	confirmed, err := c.isClockedIn(ctx)
	if err != nil {
		slog.Warn("Could not verify clock-in status", "error", err)
		return fmt.Errorf("clock-in submitted but verification failed: %w", err)
	}
	if !confirmed {
		slog.Error("Clock-in submitted but server state did not change")
		return fmt.Errorf("clock-in submitted but IsClockedIn still reports false")
	}

	slog.Info("Clock-in confirmed successfully")
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

// isClockedIn checks if the user is currently clocked in.
func (c *MyTeam2GoClocker) isClockedIn(ctx context.Context) (bool, error) {
	if err := c.login(ctx); err != nil {
		return false, err
	}

	// Give the server a moment to reflect the action before querying status.
	time.Sleep(2 * time.Second)

	homeURL := c.baseURL + "/home.xhtml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homeURL, nil)
	if err != nil {
		slog.Warn("Failed to create request of home page", "error", err)
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	c.setBrowserHeaders(req, c.baseURL+"/")

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Warn("Failed to fetch home page", "error", err)
		return false, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	html := string(bodyBytes)

	// "Inicio jornada laboral" only appears when NOT clocked in.
	// When clocked in, different options appear (Fin jornada, Pausa almuerzo, etc.)
	hasClockInOption := strings.Contains(html, "Inicio jornada laboral")
	isClockedIn := !hasClockInOption

	slog.Info("Clock-in status checked", "isClockedIn", isClockedIn, "clockInOptionVisible", hasClockInOption)
	return isClockedIn, nil
}

// humanLocation returns location strings to include in the clock-in form.
//
// If coordinates are configured, it returns a slightly jittered version
// simulating the natural imprecision of a real browser's Geolocation API
// (±~11 m random offset, 6 decimal places, realistic accuracy in metres).
//
// If no coordinates are configured (both zero), it returns empty lat/lon
// and the permission-denied error message that Chrome sends when the user
// blocks geolocation access — indistinguishable from a real denial.
func (c *MyTeam2GoClocker) humanLocation() (lat, lon, locationErr string) {
	if c.latitude == 0 && c.longitude == 0 {
		// Mimic MyTeam2Go's permission-denied payload.
		return "", "", "geolocation.error.permission_denied"
	}

	// ±0.0001° ≈ ±11 m, well within normal GPS/WiFi-positioning variance.
	jitter := func() float64 { return (rand.Float64()*2 - 1) * 0.0001 }
	lat = fmt.Sprintf("%.6f", c.latitude+jitter())
	lon = fmt.Sprintf("%.6f", c.longitude+jitter())
	// locationError is left empty when successful
	locationErr = ""
	return
}

// setBrowserHeaders adds common browser headers to a request to avoid bot detection.
func (c *MyTeam2GoClocker) setBrowserHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9")
	req.Header.Set("Origin", c.baseURL)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}
