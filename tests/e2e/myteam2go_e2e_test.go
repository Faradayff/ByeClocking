//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faradayff/ByeClocking/internal/clients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fake MyTeam2Go server
// ---------------------------------------------------------------------------

type serverState int

const (
	stateLoggedOut serverState = iota
	stateClockedIn
	statePaused
	stateClockedOut
)

type fakeMyTeam2GoServer struct {
	server *httptest.Server
	mu     sync.Mutex
	state  serverState

	// Fault injection
	failLogin        bool // login redirects to ?error=true
	rejectNextAction bool // Guardar returns "No se ha podido efectuar"
	// Whether to include a valid JSESSIONID cookie on login
	omitSessionCookie bool
	// When set, the POST /home.xhtml (Guardar) does NOT flip state → verification fails
	freezeState bool
}

func newFakeServer(t *testing.T) *fakeMyTeam2GoServer {
	t.Helper()
	fs := &fakeMyTeam2GoServer{}
	mux := http.NewServeMux()

	// ── POST /j_security_check ────────────────────────────────────────────
	mux.HandleFunc("/j_security_check", func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		defer fs.mu.Unlock()

		if fs.failLogin {
			http.Redirect(w, r, "/login?error=true", http.StatusFound)
			return
		}
		if fs.omitSessionCookie {
			http.Redirect(w, r, "/home.xhtml", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "fake-session-abc"})
		http.Redirect(w, r, "/home.xhtml", http.StatusFound)
	})

	// ── GET /home.xhtml & POST /home.xhtml (all AJAX) ────────────────────
	mux.HandleFunc("/home.xhtml", func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		state := fs.state
		reject := fs.rejectNextAction
		freeze := fs.freezeState
		fs.mu.Unlock()

		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			fmt.Fprint(w, fs.buildHomeHTML(state))
			return
		}

		// POST — decide which AJAX action this is
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		source := r.FormValue("jakarta.faces.source")
		btnName := "workAssistanceForm:j_idt999"

		switch {
		// ── Menu click (load work assistance form) ──────────────────────
		case strings.HasPrefix(source, "topMenuIdForm:menuWork"):
			w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="workAssistanceForm"><![CDATA[%s]]></update>
<update id="j_id1:jakarta.faces.ViewState:1"><![CDATA[updated-vs-456]]></update>
</changes>
</partial-response>`, fs.buildWorkAssistanceFormHTML(state, btnName))

		// ── Calendar widget ─────────────────────────────────────────────
		case strings.HasPrefix(source, "homePartialLoadings:"):
			today := time.Now()
			// Return a vacation that includes today (for holiday tests)
			startDate := today.AddDate(0, 0, -1).Format("02/01/2006")
			endDate := today.AddDate(0, 0, 1).Format("02/01/2006")
			w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="homeForm:calendarWidgetContent"><![CDATA[
<div>Vacaciones del %s a %s</div>
]]></update>
</changes>
</partial-response>`, startDate, endDate)

		// ── Change event (dropdown selection) ───────────────────────────
		case source == "workAssistanceForm:inputOption" || source == "workAssistanceForm:outputOption":
			w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="workAssistanceForm:workAssistanceFormContent"><![CDATA[<div>form updated</div>]]></update>
<update id="j_id1:jakarta.faces.ViewState:1"><![CDATA[change-vs-789]]></update>
</changes>
</partial-response>`)

		// ── Guardar submit ───────────────────────────────────────────────
		case source == btnName:
			if reject {
				fs.mu.Lock()
				fs.rejectNextAction = false
				fs.mu.Unlock()
				w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
				fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="messages"><![CDATA[No se ha podido efectuar el registro]]></update>
</changes>
</partial-response>`)
				return
			}

			if !freeze {
				fs.mu.Lock()
				// Advance state based on the selected option value:
				// outputOption value=2 → Inicio Pausa (Pause)
				// outputOption value=4 → Fin de jornada laboral (ClockOut)
				// inputOption  value=1 → Inicio jornada laboral (ClockIn)
				// inputOption  value=3 → Reanudar jornada laboral (Resume)
				outputOptVal := r.FormValue("workAssistanceForm:outputOption_input")
				inputOptVal := r.FormValue("workAssistanceForm:inputOption_input")
				switch {
				case inputOptVal == "1":
					fs.state = stateClockedIn
				case outputOptVal == "2":
					fs.state = statePaused
				case inputOptVal == "3":
					fs.state = stateClockedIn
				case outputOptVal == "4":
					fs.state = stateClockedOut
				}
				fs.mu.Unlock()
			}

			w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="messages"><![CDATA[<div>OK</div>]]></update>
</changes>
</partial-response>`)

		default:
			http.Error(w, "unknown source: "+source, http.StatusBadRequest)
		}
	})

	fs.server = httptest.NewServer(mux)
	t.Cleanup(fs.server.Close)
	return fs
}

// buildHomeHTML returns an HTML page that satisfies all regex used in myteam2go.go.
func (fs *fakeMyTeam2GoServer) buildHomeHTML(state serverState) string {
	var options string
	switch state {
	case stateLoggedOut:
		options = `<option value="1">Inicio jornada laboral</option>`
	case stateClockedIn:
		options = `<option value="2">Inicio Pausa</option><option value="4">Fin de jornada laboral</option>`
	case statePaused:
		options = `<option value="3">Reanudar jornada laboral</option>`
	case stateClockedOut:
		// No clock-in option visible → already clocked out
		options = ``
	}

	today := time.Now()
	startDate := today.AddDate(0, 0, -1).Format("02/01/2006")
	endDate := today.AddDate(0, 0, 1).Format("02/01/2006")
	_ = startDate
	_ = endDate

	return fmt.Sprintf(`<!DOCTYPE html><html><body>
<input name="jakarta.faces.ViewState" value="home-vs-111">
<div id="topMenuIdForm:menuWork1"></div>
<script id="homePartialLoadings:j_idtCAL">
loadCalendaryWidget = function() {
    return PrimeFaces.ab({s:"homePartialLoadings:j_idtCAL", f:"homePartialLoadings",
                          u:"homeForm:calendarWidgetContent", a:true});
}
</script>
<form id="workAssistanceForm">
%s
<button name="workAssistanceForm:j_idt999"><span>Guardar</span></button>
</form>
</body></html>`, options)
}

// buildWorkAssistanceFormHTML returns the AJAX fragment with the work assistance form.
func (fs *fakeMyTeam2GoServer) buildWorkAssistanceFormHTML(state serverState, btnName string) string {
	var options string
	switch state {
	case stateLoggedOut:
		options = `<option value="1">Inicio jornada laboral</option>`
	case stateClockedIn:
		options = `<option value="2">Inicio Pausa</option><option value="4">Fin de jornada laboral</option>`
	case statePaused:
		options = `<option value="3">Reanudar jornada laboral</option>`
	case stateClockedOut:
		options = ``
	}

	return fmt.Sprintf(`<form id="workAssistanceForm">
<input name="jakarta.faces.ViewState" value="menu-vs-222">
%s
<button name="%s"><span>Guardar</span></button>
</form>`, options, btnName)
}

// newClocker creates a MyTeam2GoClocker pointed at the fake server.
// We need to set the baseURL to the fake server, which requires constructing
// the clocker and swapping its http.Client.
func (fs *fakeMyTeam2GoServer) newClocker() *clients.MyTeam2GoClocker {
	// Extract just the host from the server URL (no scheme)
	host := strings.TrimPrefix(fs.server.URL, "http://")
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Jar: jar,
		// Follow redirects (default behaviour)
	}
	return clients.NewMyTeam2GoTestClocker(host, "testuser", "testpass", 0, 0, httpClient)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestMyTeam2Go_FullDayFlow(t *testing.T) {
	fs := newFakeServer(t)
	clocker := fs.newClocker()
	ctx := context.Background()

	require.NoError(t, clocker.ClockIn(ctx), "ClockIn should succeed")
	assert.Equal(t, stateClockedIn, fs.state, "state should be ClockedIn after ClockIn")

	require.NoError(t, clocker.ClockPause(ctx), "ClockPause should succeed")
	assert.Equal(t, statePaused, fs.state, "state should be Paused after ClockPause")

	require.NoError(t, clocker.ClockResume(ctx), "ClockResume should succeed")
	assert.Equal(t, stateClockedIn, fs.state, "state should be ClockedIn after ClockResume")

	require.NoError(t, clocker.ClockOut(ctx), "ClockOut should succeed")
	assert.Equal(t, stateClockedOut, fs.state, "state should be ClockedOut after ClockOut")
}

func TestMyTeam2Go_LoginFailed(t *testing.T) {
	fs := newFakeServer(t)
	fs.failLogin = true
	clocker := fs.newClocker()
	ctx := context.Background()

	err := clocker.ClockIn(ctx)
	require.Error(t, err, "ClockIn should fail when login fails")
}

func TestMyTeam2Go_LoginMissingSessionCookie(t *testing.T) {
	fs := newFakeServer(t)
	fs.omitSessionCookie = true
	clocker := fs.newClocker()
	ctx := context.Background()

	err := clocker.ClockIn(ctx)
	require.Error(t, err, "ClockIn should fail when JSESSIONID cookie is absent")
	assert.Contains(t, err.Error(), "JSESSIONID")
}

func TestMyTeam2Go_ClockInAlreadyClockedIn(t *testing.T) {
	fs := newFakeServer(t)
	fs.state = stateClockedIn // already clocked in
	clocker := fs.newClocker()
	ctx := context.Background()

	// ClockIn should detect isReadyTo==false and skip silently (no error).
	require.NoError(t, clocker.ClockIn(ctx))
	// State should not change
	assert.Equal(t, stateClockedIn, fs.state)
}

func TestMyTeam2Go_ServerRejectsAction(t *testing.T) {
	fs := newFakeServer(t)
	fs.rejectNextAction = true
	clocker := fs.newClocker()
	ctx := context.Background()

	err := clocker.ClockIn(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no se ha podido efectuar")
}

func TestMyTeam2Go_ClockInVerificationFails(t *testing.T) {
	fs := newFakeServer(t)
	// State won't change after Guardar → verification will find ClockOut not ready
	fs.freezeState = true
	clocker := fs.newClocker()
	ctx := context.Background()

	err := clocker.ClockIn(ctx)
	require.Error(t, err)
	// The real error from myteam2go.go when post-submit verification fails:
	// "clock-in submitted but isReadyToClockOut still reports false"
	assert.Contains(t, err.Error(), "clock-in submitted but")
}

func TestMyTeam2Go_IsHoliday_True(t *testing.T) {
	fs := newFakeServer(t)
	clocker := fs.newClocker()
	ctx := context.Background()

	// The fake calendar panel returns a vacation that covers today
	isHoliday := clocker.IsHoliday(ctx)
	assert.True(t, isHoliday, "today should be detected as a holiday")
}

func TestMyTeam2Go_IsHoliday_False(t *testing.T) {
	// Build a dedicated server that returns no vacation data in the calendar panel
	mux := http.NewServeMux()
	mux.HandleFunc("/j_security_check", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "fake-session-xyz"})
		http.Redirect(w, r, "/home.xhtml", http.StatusFound)
	})
	mux.HandleFunc("/home.xhtml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, noHolidayHomeHTML())
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		// Calendar AJAX returns no vacations
		w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="homeForm:calendarWidgetContent"><![CDATA[<div>No hay vacaciones</div>]]></update>
</changes>
</partial-response>`)
	})

	noHolidayServer := httptest.NewServer(mux)
	t.Cleanup(noHolidayServer.Close)

	host := strings.TrimPrefix(noHolidayServer.URL, "http://")
	jar, _ := cookiejar.New(nil)
	clocker := clients.NewMyTeam2GoTestClocker(host, "u", "p", 0, 0, &http.Client{Jar: jar})

	isHoliday := clocker.IsHoliday(context.Background())
	assert.False(t, isHoliday, "today should NOT be detected as a holiday")
}

func noHolidayHomeHTML() string {
	return `<!DOCTYPE html><html><body>
<input name="jakarta.faces.ViewState" value="home-vs-111">
<div id="topMenuIdForm:menuWork1"></div>
<script id="homePartialLoadings:j_idtCAL">
loadCalendaryWidget = function() {
    return PrimeFaces.ab({s:"homePartialLoadings:j_idtCAL", f:"homePartialLoadings",
                          u:"homeForm:calendarWidgetContent", a:true});
}
</script>
</body></html>`
}
