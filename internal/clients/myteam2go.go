package clients

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

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

// NewMyTeam2GoTestClocker creates a MyTeam2GoClocker that targets an arbitrary
// host (scheme + host, e.g. "127.0.0.1:PORT") using the provided http.Client.
// Intended only for tests; the baseURL is set to "http://<host>" so that a
// local httptest.Server can be used instead of the real MyTeam2Go website.
func NewMyTeam2GoTestClocker(host, username, password string, latitude, longitude float64, client *http.Client) *MyTeam2GoClocker {
	return &MyTeam2GoClocker{
		baseURL:   "http://" + host,
		username:  username,
		password:  password,
		latitude:  latitude,
		longitude: longitude,
		client:    client,
	}
}

// workAssistanceAction describes a clock action to submit via the workAssistanceForm.
type workAssistanceAction struct {
	// optionLabel is the visible text of the option to select (e.g. "Inicio jornada laboral").
	optionLabel string
	// optionField is the JSF component name that holds the selected value.
	// When not clocked in, the server renders "inputOption"; once clocked in, it renders "outputOption".
	optionField string
	// logVerb is used in log messages to describe the action.
	logVerb string
}

var (
	actionClockIn  = workAssistanceAction{"Inicio jornada laboral", "inputOption", "clock-in"}
	actionPause    = workAssistanceAction{"Inicio Pausa", "outputOption", "clock-pause"}
	actionResume   = workAssistanceAction{"Reanudar jornada laboral", "inputOption", "clock-resume"}
	actionClockOut = workAssistanceAction{"Fin de jornada laboral", "outputOption", "clock-out"}
)

// ClockIn sends a clock-in request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockIn(ctx context.Context) error {
	if err := c.Login(ctx); err != nil {
		slog.Error("❌ Error logging in. Impossible to clock in", "error", err)
		return err
	}

	isReadyTo, err := c.isReadyTo(ctx, actionClockIn)
	if err != nil {
		slog.Warn("⚠️ Could not verify clock-in status before attempting", "error", err)
	}
	if !isReadyTo {
		slog.Debug("⏭️ Already clocked in, skipping")
		return nil
	}

	if err := c.submitWorkAssistance(ctx, actionClockIn); err != nil {
		slog.Error("❌ Clock-in failed", "error", err)
		return err
	}

	time.Sleep(2 * time.Second)
	isReadyTo, err = c.isReadyTo(ctx, actionClockOut)
	if err != nil {
		return fmt.Errorf("clock-in submitted but verification failed: %w", err)
	}
	if !isReadyTo {
		return fmt.Errorf("clock-in submitted but isReadyToClockOut still reports false")
	}

	slog.Debug("✅ Clock-in confirmed successfully")
	return nil
}

// ClockOut sends a clock-out request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockOut(ctx context.Context) error {
	if err := c.Login(ctx); err != nil {
		slog.Error("❌ Error logging in. Impossible to clock out", "error", err)
		return err
	}

	isReadyTo, err := c.isReadyTo(ctx, actionClockOut)
	if err != nil {
		slog.Warn("⚠️ Could not verify clock-in status before attempting", "error", err)
	}
	if !isReadyTo {
		slog.Debug("⏭️ Already clocked out, skipping")
		return nil
	}

	if err := c.submitWorkAssistance(ctx, actionClockOut); err != nil {
		slog.Error("❌ Clock-out failed", "error", err)
		return err
	}

	time.Sleep(2 * time.Second)
	isReadyTo, err = c.isReadyTo(ctx, actionClockOut)
	if err != nil {
		return fmt.Errorf("clock-out submitted but verification failed: %w", err)
	}
	if isReadyTo {
		return fmt.Errorf("clock-out submitted but isReadyToClockOut still reports true")
	}

	slog.Debug("✅ Clock-out submitted successfully")
	return nil
}

// ClockPause sends a lunch-pause request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockPause(ctx context.Context) error {
	if err := c.Login(ctx); err != nil {
		slog.Error("❌ Error logging in. Impossible to clock pause", "error", err)
		return err
	}

	isReadyTo, err := c.isReadyTo(ctx, actionPause)
	if err != nil {
		slog.Warn("⚠️ Could not verify clock-in status before attempting", "error", err)
	}
	if !isReadyTo {
		slog.Debug("⏭️ Already in pause, skipping")
		return nil
	}

	if err := c.submitWorkAssistance(ctx, actionPause); err != nil {
		slog.Error("❌ Clock-pause failed", "error", err)
		return err
	}

	time.Sleep(2 * time.Second)
	isReadyTo, err = c.isReadyTo(ctx, actionResume)
	if err != nil {
		return fmt.Errorf("clock-pause submitted but verification failed: %w", err)
	}
	if !isReadyTo {
		return fmt.Errorf("clock-pause submitted but isReadyToResume still reports false")
	}

	slog.Debug("✅ Clock-pause submitted successfully")
	return nil
}

// ClockResume sends a resume-from-lunch request to MyTeam2Go.
func (c *MyTeam2GoClocker) ClockResume(ctx context.Context) error {
	if err := c.Login(ctx); err != nil {
		slog.Error("❌ Error logging in. Impossible to clock resume", "error", err)
		return err
	}

	isReadyTo, err := c.isReadyTo(ctx, actionResume)
	if err != nil {
		slog.Warn("⚠️ Could not verify clock-resume status before attempting", "error", err)
	}
	if !isReadyTo {
		slog.Debug("⏭️ Already clocked back, skipping")
		return nil
	}

	if err := c.submitWorkAssistance(ctx, actionResume); err != nil {
		slog.Error("❌ Clock-resume failed", "error", err)
		return err
	}

	time.Sleep(2 * time.Second)
	isReadyTo, err = c.isReadyTo(ctx, actionClockOut)
	if err != nil {
		return fmt.Errorf("clock-resume submitted but verification failed: %w", err)
	}
	if !isReadyTo {
		return fmt.Errorf("clock-resume submitted but isReadyToClockOut still reports false")
	}

	slog.Debug("✅ Clock-resume submitted successfully")
	return nil
}

// IsHoliday checks if the current day is a vacation day by inspecting the approved
// vacation requests shown on the MyTeam2Go home page.
// The vacation calendar panel is loaded dynamically via AJAX, so after fetching the
// initial home page we trigger the calendar panel the same way the browser does.
func (c *MyTeam2GoClocker) IsHoliday(ctx context.Context) bool {
	if err := c.Login(ctx); err != nil {
		slog.Error("❌ IsHoliday: login failed, assuming not a holiday", "error", err)
		return false
	}

	homeURL := c.baseURL + "/home.xhtml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homeURL, nil)
	if err != nil {
		slog.Error("❌ IsHoliday: failed to create request", "error", err)
		return false
	}
	c.setBrowserHeaders(req, c.baseURL+"/")

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("❌ IsHoliday: failed to fetch home page", "error", err)
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("⚠️ IsHoliday: failed to close response body", "error", err)
		}
	}()
	bodyBytes, _ := io.ReadAll(resp.Body)
	homeHTML := string(bodyBytes)

	today := time.Now()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	vacationRegex := regexp.MustCompile(`Vacaciones del\s+(\d{2}/\d{2}/\d{4})\s+a\s+(\d{2}/\d{2}/\d{4})`)

	// The calendar section is rendered via a JSF AJAX partial request.
	// Find the calendar topbar button (e.g. topMenuIdForm:menuCalendar...) and trigger it.
	calendarHTML, err := c.loadCalendarPanel(ctx, homeHTML, homeURL)
	if err != nil {
		slog.Warn("⚠️ IsHoliday: could not load calendar panel via AJAX, falling back to home HTML", "error", err)
		calendarHTML = homeHTML
	}

	matches := vacationRegex.FindAllStringSubmatch(calendarHTML, -1)
	slog.Debug("🏖️ IsHoliday: vacation matches found", "count", len(matches), "matches", matches)

	for _, m := range matches {
		startDate, err1 := time.ParseInLocation("02/01/2006", m[1], today.Location())
		endDate, err2 := time.ParseInLocation("02/01/2006", m[2], today.Location())
		if err1 != nil || err2 != nil {
			slog.Warn("⚠️ IsHoliday: failed to parse vacation dates", "start", m[1], "end", m[2])
			continue
		}
		if !todayDate.Before(startDate) && !todayDate.After(endDate) {
			slog.Info("🏖️ IsHoliday: today is within an approved vacation period", "start", m[1], "end", m[2])
			return true
		}
	}

	slog.Debug("✅ IsHoliday: no vacation period matches today")
	return false
}

// loadCalendarPanel triggers the calendar widget on the MyTeam2Go home page via a JSF
// partial-AJAX request. The browser fires this automatically on page load via the
// JavaScript function loadCalendaryWidget(), which is defined in a <script> tag like:
//
//	<script id="homePartialLoadings:j_idtXXXX">
//	    loadCalendaryWidget = function() {
//	        return PrimeFaces.ab({s:"homePartialLoadings:j_idtXXXX", f:"homePartialLoadings",
//	                              u:"homeForm:calendarWidgetContent", a:true, ...});
//	    }
//	</script>
//
// We extract the dynamic source ID from that script and fire the equivalent POST.
func (c *MyTeam2GoClocker) loadCalendarPanel(ctx context.Context, homeHTML, homeURL string) (string, error) {
	// ViewState helpers (same as in submitWorkAssistance).
	viewStateRegex := regexp.MustCompile(`name="jakarta\.faces\.ViewState"[^>]*value="([^"]+)"`)
	viewStateCDATARegex := regexp.MustCompile(`<update id="[^"]*ViewState[^"]*"><!\[CDATA\[([^]]+)]]></update>`)
	extractViewState := func(body string) string {
		if m := viewStateRegex.FindStringSubmatch(body); len(m) >= 2 {
			return m[1]
		}
		if m := viewStateCDATARegex.FindStringSubmatch(body); len(m) >= 2 {
			return m[1]
		}
		return ""
	}

	viewState := extractViewState(homeHTML)
	if viewState == "" {
		return "", fmt.Errorf("could not find ViewState on home page")
	}

	// Find the source component ID for loadCalendaryWidget.
	// The page contains a script like:
	//   loadCalendaryWidget = function() {return PrimeFaces.ab({s:"homePartialLoadings:j_idtXXXX",...});}
	calendarSrcRegex := regexp.MustCompile(`loadCalendaryWidget\s*=\s*function[^{]*\{[^}]*s:\s*"(homePartialLoadings:[^"]+)"`)
	srcMatches := calendarSrcRegex.FindStringSubmatch(homeHTML)
	if len(srcMatches) < 2 {
		return "", fmt.Errorf("could not find loadCalendaryWidget source ID in home HTML")
	}
	srcID := srcMatches[1]

	menuData := url.Values{}
	menuData.Set("jakarta.faces.partial.ajax", "true")
	menuData.Set("jakarta.faces.source", srcID)
	menuData.Set("jakarta.faces.partial.execute", srcID)
	menuData.Set("jakarta.faces.partial.render", "homeForm:calendarWidgetContent")
	menuData.Set(srcID, srcID)
	menuData.Set("homePartialLoadings", "homePartialLoadings")
	menuData.Set("jakarta.faces.ViewState", viewState)

	menuReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(menuData.Encode()))
	menuReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	menuReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	menuReq.Header.Set("Faces-Request", "partial/ajax")
	menuReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(menuReq, c.baseURL+"/home")

	menuResp, err := c.client.Do(menuReq)
	if err != nil {
		return "", fmt.Errorf("AJAX request for calendar panel failed: %w", err)
	}
	defer func() {
		if err := menuResp.Body.Close(); err != nil {
			slog.Warn("⚠️ loadCalendarPanel: failed to close menu response body", "error", err)
		}
	}()
	menuBytes, _ := io.ReadAll(menuResp.Body)
	return string(menuBytes), nil
}

// submitWorkAssistance executes the full workAssistanceForm flow for the given action:
//  1. GET the home page and extract ViewState.
//  2. POST the topbar "Mi control horario" button to load the workAssistanceForm dialog.
//  3. POST the change event to select the desired option.
//  4. POST "Guardar" to submit the form.
func (c *MyTeam2GoClocker) submitWorkAssistance(ctx context.Context, action workAssistanceAction) error {
	homeURL := c.baseURL + "/home.xhtml"
	homeReferer := c.baseURL + "/home"

	// ── GET home page ─────────────────────────────────────────────────────────
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create home request: %w", err)
	}
	c.setBrowserHeaders(req, c.baseURL+"/")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch home page: %w", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := resp.Body.Close(); err != nil {
		slog.Warn("⚠️ submitWorkAssistance: failed to close home response body", "error", err)
	}
	html := string(bodyBytes)

	// ── ViewState helpers ─────────────────────────────────────────────────────
	viewStateRegex := regexp.MustCompile(`name="jakarta\.faces\.ViewState"[^>]*value="([^"]+)"`)
	viewStateCDATARegex := regexp.MustCompile(`<update id="[^"]*ViewState[^"]*"><!\[CDATA\[([^]]+)]]></update>`)

	extractViewState := func(body string) string {
		if m := viewStateRegex.FindStringSubmatch(body); len(m) >= 2 {
			return m[1]
		}
		if m := viewStateCDATARegex.FindStringSubmatch(body); len(m) >= 2 {
			return m[1]
		}
		return ""
	}

	viewState := extractViewState(html)
	if viewState == "" {
		return fmt.Errorf("could not find ViewState on home page")
	}

	// ── Step 1: Click the topbar "Mi control horario" button ─────────────────────
	menuHtml, newViewState, err := c.loadWorkAssistanceMenu(ctx, html, homeURL, viewState)
	if err != nil {
		return err
	}
	if newViewState != "" {
		viewState = newViewState
		slog.Debug("🔄 ViewState updated from menu response")
	}

	// ── Extract Guardar button name ───────────────────────────────────────────
	btnRegex := regexp.MustCompile(`name="(workAssistanceForm:j_idt\d+)"[^>]*><span[^>]*>Guardar</span>`)
	btnMatches := btnRegex.FindStringSubmatch(menuHtml)
	if len(btnMatches) < 2 {
		btnMatches = btnRegex.FindStringSubmatch(html)
		if len(btnMatches) < 2 {
			return fmt.Errorf("could not find Guardar button in workAssistanceForm")
		}
	}
	btnName := btnMatches[1]

	// ── Extract option value ──────────────────────────────────────────────────
	optRegex := regexp.MustCompile(`value="(\d+)"[^>]*>` + regexp.QuoteMeta(action.optionLabel) + `<`)
	optMatches := optRegex.FindStringSubmatch(menuHtml)
	if len(optMatches) < 2 {
		optMatches = optRegex.FindStringSubmatch(html)
		if len(optMatches) < 2 {
			return fmt.Errorf("could not find option '%s'", action.optionLabel)
		}
	}
	optValue := optMatches[1]
	slog.Debug("🔍 Resolved action parameters", "action", action.logVerb, "btnName", btnName, "optionField", action.optionField, "optValue", optValue)

	// ── Step 2: Simulate the change event on the dropdown ─────────────────────
	time.Sleep(300 * time.Millisecond)

	optionComponent := "workAssistanceForm:" + action.optionField
	changeData := url.Values{}
	changeData.Set("jakarta.faces.partial.ajax", "true")
	changeData.Set("jakarta.faces.source", optionComponent)
	changeData.Set("jakarta.faces.partial.execute", optionComponent)
	changeData.Set("jakarta.faces.partial.render", "workAssistanceForm:workAssistanceFormContent")
	changeData.Set("jakarta.faces.behavior.event", "change")
	changeData.Set("jakarta.faces.partial.event", "change")
	changeData.Set("workAssistanceForm", "workAssistanceForm")
	changeData.Set("workAssistanceForm:"+action.optionField+"_input", optValue)
	changeData.Set("workAssistanceForm:locationLatitude", "")
	changeData.Set("workAssistanceForm:locationLongitude", "")
	changeData.Set("workAssistanceForm:locationError", "")
	changeData.Set("jakarta.faces.ViewState", viewState)

	changeReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(changeData.Encode()))
	changeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	changeReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	changeReq.Header.Set("Faces-Request", "partial/ajax")
	changeReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(changeReq, homeReferer)

	changeResp, err := c.client.Do(changeReq)
	if err != nil {
		return fmt.Errorf("failed to execute change event: %w", err)
	}
	changeBytes, _ := io.ReadAll(changeResp.Body)
	changeHtml := string(changeBytes)
	slog.Debug("🌐 Change event response", "body", changeHtml)
	if err := changeResp.Body.Close(); err != nil {
		slog.Warn("⚠️ submitWorkAssistance: failed to close change response body", "error", err)
	}

	if vs := extractViewState(changeHtml); vs != "" {
		viewState = vs
		slog.Debug("🔄 ViewState updated from change response")
	}

	// NOTE: We intentionally skip the updateLocationForm intermediate request.
	// That call uses ps:true (process @form), which triggers full JSF validation
	// server-side and fails because the bean state isn't ready yet.
	// The browser fires it as a background geolocation update, but the coordinates
	// are also included in the final "Guardar" submit — which is the only one that matters.
	lat, lon, locationErr := humanLocation(c.latitude, c.longitude)

	// ── Step 3: Submit "Guardar" ──────────────────────────────────────────────
	submitData := url.Values{}
	submitData.Set("jakarta.faces.partial.ajax", "true")
	submitData.Set("jakarta.faces.source", btnName)
	submitData.Set("jakarta.faces.partial.execute", "@all")
	submitData.Set("jakarta.faces.partial.render", "workAssistanceForm messages session_messages workAssistanceForm:WAMessagesDialog")
	submitData.Set(btnName, btnName)
	submitData.Set("workAssistanceForm", "workAssistanceForm")
	submitData.Set("workAssistanceForm:"+action.optionField+"_input", optValue)
	submitData.Set("workAssistanceForm:locationLatitude", lat)
	submitData.Set("workAssistanceForm:locationLongitude", lon)
	submitData.Set("workAssistanceForm:locationError", locationErr)
	submitData.Set("jakarta.faces.ViewState", viewState)

	slog.Debug("📤 Submitting Guardar", "action", action.logVerb, "viewState", viewState, "btnName", btnName, "optValue", optValue)

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(submitData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create submit request: %w", err)
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	postReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	postReq.Header.Set("Faces-Request", "partial/ajax")
	postReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(postReq, homeReferer)

	postResp, err := c.client.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to execute submit request: %w", err)
	}
	defer func() {
		if err := postResp.Body.Close(); err != nil {
			slog.Warn("⚠️ submitWorkAssistance: failed to close post response body", "error", err)
		}
	}()

	respBody, _ := io.ReadAll(postResp.Body)
	respStr := string(respBody)
	slog.Debug("🌐 Guardar response", "action", action.logVerb, "body", respStr)

	if strings.Contains(respStr, "No se ha podido efectuar") {
		return fmt.Errorf("%s rejected by server: no se ha podido efectuar el registro", action.logVerb)
	}

	return nil
}

// Login authenticates the user by sending a POST request to the Login endpoint with username and password credentials.
// It stores the JSESSIONID cookie for session management. Returns an error if Login fails.
func (c *MyTeam2GoClocker) Login(ctx context.Context) error {
	loginURL := c.baseURL + "/j_security_check"

	credentials := url.Values{}
	credentials.Set("username", c.username)
	credentials.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(credentials.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	slog.Debug("🔑 Attempting login to MyTeam2Go", "url", loginURL, "username", c.username)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("⚠️ Login: failed to close response body", "error", err)
		}
	}()

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

	slog.Debug("🔓 Login successful", "JSESSIONID", token)
	return nil
}

// isReadyTo checks if the user is in a state to perform the action by fetching the home page
// and checking whether the action option is present in the form.
func (c *MyTeam2GoClocker) isReadyTo(ctx context.Context, status workAssistanceAction) (bool, error) {
	homeURL := c.baseURL + "/home.xhtml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homeURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	c.setBrowserHeaders(req, c.baseURL+"/")

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	html := string(bodyBytes)

	isReadyTo := strings.Contains(html, status.optionLabel)
	if !isReadyTo {
		// Try to fetch the work assistance form via AJAX if not found in the initial HTML
		viewStateRegex := regexp.MustCompile(`name="jakarta\.faces\.ViewState"[^>]*value="([^"]+)"`)
		viewStateCDATARegex := regexp.MustCompile(`<update id="[^"]*ViewState[^"]*"><!\[CDATA\[([^]]+)]]></update>`)

		extractViewState := func(body string) string {
			if m := viewStateRegex.FindStringSubmatch(body); len(m) >= 2 {
				return m[1]
			}
			if m := viewStateCDATARegex.FindStringSubmatch(body); len(m) >= 2 {
				return m[1]
			}
			return ""
		}

		viewState := extractViewState(html)
		if viewState != "" {
			if menuHtml, _, err := c.loadWorkAssistanceMenu(ctx, html, homeURL, viewState); err == nil {
				isReadyTo = strings.Contains(menuHtml, status.optionLabel)
			}
		}
	}

	slog.Debug("🔎 Clock status checked", "status", status.optionLabel, "isReadyTo", isReadyTo)
	return isReadyTo, nil
}

// loadWorkAssistanceMenu attempts to load the dynamic work assistance menu via AJAX.
func (c *MyTeam2GoClocker) loadWorkAssistanceMenu(ctx context.Context, html, homeURL, viewState string) (string, string, error) {
	menuRegex := regexp.MustCompile(`id="(topMenuIdForm:menuWork[^"]+)"`)
	menuMatches := menuRegex.FindStringSubmatch(html)
	if len(menuMatches) < 2 {
		return "", "", fmt.Errorf("could not find 'Mi control horario' topbar button")
	}
	menuID := menuMatches[1]
	slog.Debug("🔗 Found topbar menu button", "menuID", menuID)

	menuData := url.Values{}
	menuData.Set("jakarta.faces.partial.ajax", "true")
	menuData.Set("jakarta.faces.source", menuID)
	menuData.Set("jakarta.faces.partial.execute", menuID)
	menuData.Set("jakarta.faces.partial.render", "workAssistanceForm")
	menuData.Set(menuID, menuID)
	menuData.Set("topMenuIdForm", "topMenuIdForm")
	menuData.Set("jakarta.faces.ViewState", viewState)

	menuReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, homeURL, strings.NewReader(menuData.Encode()))
	menuReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	menuReq.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	menuReq.Header.Set("Faces-Request", "partial/ajax")
	menuReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	c.setBrowserHeaders(menuReq, c.baseURL+"/home")

	menuResp, err := c.client.Do(menuReq)
	if err != nil {
		return "", "", fmt.Errorf("failed to click 'Mi control horario': %w", err)
	}
	defer func() {
		if err := menuResp.Body.Close(); err != nil {
			slog.Warn("⚠️ loadWorkAssistanceMenu: failed to close menu response body", "error", err)
		}
	}()
	menuBytes, _ := io.ReadAll(menuResp.Body)
	menuHtml := string(menuBytes)

	viewStateRegex := regexp.MustCompile(`name="jakarta\.faces\.ViewState"[^>]*value="([^"]+)"`)
	viewStateCDATARegex := regexp.MustCompile(`<update id="[^"]*ViewState[^"]*"><!\[CDATA\[([^]]+)]]></update>`)

	extractViewState := func(body string) string {
		if m := viewStateRegex.FindStringSubmatch(body); len(m) >= 2 {
			return m[1]
		}
		if m := viewStateCDATARegex.FindStringSubmatch(body); len(m) >= 2 {
			return m[1]
		}
		return ""
	}

	newViewState := extractViewState(menuHtml)
	return menuHtml, newViewState, nil
}

// setBrowserHeaders adds common browser headers to a request to avoid bot detection.
func (c *MyTeam2GoClocker) setBrowserHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	req.Header.Set("Origin", c.baseURL)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}
