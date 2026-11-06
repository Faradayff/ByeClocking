//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
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

	// ── GET /pages/employeePortal/calendarEmployee.xhtml ─────────────────
	mux.HandleFunc("/pages/employeePortal/calendarEmployee.xhtml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		currentYear := time.Now().Year()
		fmt.Fprintf(w, `<div class="calendar-header panel panel-default">
<table>
<th class="prev"><span class="glyphicon glyphicon-chevron-left"></span></th>
<th class="year-title">%d</th>
<th class="next disabled"><span class="glyphicon glyphicon-chevron-right"></span></th>
</table>
</div>
<div class="month-container col-xs-2">
<table class="month">
<thead>
<tr><th class="month-title" colspan="7">Enero</th></tr>
</thead>
<tr>
<td class="day styled-day" style="background-color: rgb(238, 119, 68); color: white;" title=""><div class="day-content">1</div></td>
<td class="day"><div class="day-content">2</div></td>
</tr>
</table>
</div>`, currentYear)
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

		// ── Ver Calendario link click ────────────────────────────────────
		case strings.HasPrefix(source, "homeForm:j_idt"):
			w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<redirect url="/pages/employeePortal/calendarEmployee.xhtml"></redirect>
</partial-response>`)

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
<a id="homeForm:j_idt1407" href="#" class="ui-commandlink ui-widget" onclick="PrimeFaces.ab({s:&quot;homeForm:j_idt1407&quot;});">Ver Calendario&gt;</a>
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

func TestMyTeam2Go_LoginFailed(t *testing.T) {
	fs := newFakeServer(t)
	fs.failLogin = true
	clocker := fs.newClocker()
	ctx := context.Background()

	err := clocker.Login(ctx)
	require.Error(t, err, "Login should fail when login fails")
}

func TestMyTeam2Go_LoginMissingSessionCookie(t *testing.T) {
	fs := newFakeServer(t)
	fs.omitSessionCookie = true
	clocker := fs.newClocker()
	ctx := context.Background()

	err := clocker.Login(ctx)
	require.Error(t, err, "Login should fail when JSESSIONID cookie is absent")
	assert.Contains(t, err.Error(), "JSESSIONID")
}

func TestMyTeam2Go_IsHoliday_True(t *testing.T) {
	_ = os.RemoveAll("cache")
	defer func() { _ = os.RemoveAll("cache") }()

	fs := newFakeServer(t)
	clocker := fs.newClocker()
	ctx := context.Background()

	// The fake calendar panel returns a vacation that covers today
	isHoliday := clocker.IsHoliday(ctx)
	assert.True(t, isHoliday, "today should be detected as a holiday")
}

func TestMyTeam2Go_IsHoliday_False(t *testing.T) {
	_ = os.RemoveAll("cache")
	defer func() { _ = os.RemoveAll("cache") }()

	// Build a dedicated server that returns no vacation data in the calendar panel
	mux := http.NewServeMux()
	mux.HandleFunc("/j_security_check", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "fake-session-xyz"})
		http.Redirect(w, r, "/home.xhtml", http.StatusFound)
	})
	mux.HandleFunc("/pages/employeePortal/calendarEmployee.xhtml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		currentYear := time.Now().Year()
		// Returns Jan 1 as bank holiday; today is not Jan 1 (unless run on Jan 1)
		fmt.Fprintf(w, `<div class="calendar-header panel panel-default">
<table><th class="year-title">%d</th></table>
</div>
<div class="month-container col-xs-2">
<table class="month">
<thead><tr><th class="month-title">Enero</th></tr></thead>
<tr><td class="day styled-day" style="background-color: rgb(238, 119, 68);"><div class="day-content">1</div></td></tr>
</table>
</div>`, currentYear)
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

func TestMyTeam2Go_IsHoliday_BankHoliday_FromWebAndCache(t *testing.T) {
	_ = os.RemoveAll("cache")
	defer func() { _ = os.RemoveAll("cache") }()

	mux := http.NewServeMux()
	mux.HandleFunc("/j_security_check", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "fake-session-test"})
		http.Redirect(w, r, "/home.xhtml", http.StatusFound)
	})

	today := time.Now()
	calendarHits := 0

	mux.HandleFunc("/pages/employeePortal/calendarEmployee.xhtml", func(w http.ResponseWriter, r *http.Request) {
		calendarHits++
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		var monthsHTML string
		for m := 1; m <= 12; m++ {
			if m == int(today.Month()) {
				monthsHTML += fmt.Sprintf(`<div class="month-container col-xs-2">
<table class="month">
<thead><tr><th class="month-title">CustomLang Month %d</th></tr></thead>
<tr><td class="day styled-day" style="background-color: rgb(210, 47, 50);"><div class="day-content">%d</div></td></tr>
</table>
</div>`, m, today.Day())
			} else {
				monthsHTML += fmt.Sprintf(`<div class="month-container col-xs-2">
<table class="month">
<thead><tr><th class="month-title">CustomLang Month %d</th></tr></thead>
<tr><td class="day"><div class="day-content">1</div></td></tr>
</table>
</div>`, m)
			}
		}

		fmt.Fprintf(w, `<div class="calendar-header panel panel-default">
<table><th class="year-title">%d</th></table>
</div>
%s`, today.Year(), monthsHTML)
	})

	mux.HandleFunc("/home.xhtml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, noHolidayHomeHTML())
			return
		}
		// POST click "Ver Calendario"
		w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<redirect url="/pages/employeePortal/calendarEmployee.xhtml"></redirect>
</partial-response>`)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	jar, _ := cookiejar.New(nil)
	clocker := clients.NewMyTeam2GoTestClocker(host, "u", "p", 0, 0, &http.Client{Jar: jar})

	ctx := context.Background()

	// 1st call: calendar is fetched, parsed, cached, and detects holiday
	isHoliday1 := clocker.IsHoliday(ctx)
	assert.True(t, isHoliday1, "should detect bank holiday")
	assert.Equal(t, 1, calendarHits, "calendar should be fetched once")

	// 2nd call: detected directly from cache without hitting calendarEmployee.xhtml
	isHoliday2 := clocker.IsHoliday(ctx)
	assert.True(t, isHoliday2, "should detect bank holiday from cache")
	assert.Equal(t, 1, calendarHits, "calendar should not be fetched again because cache has current year")
}

func TestMyTeam2Go_IsHoliday_NewYear_Reset(t *testing.T) {
	_ = os.RemoveAll("cache")
	defer func() { _ = os.RemoveAll("cache") }()

	// Write cache with previous year bank holiday
	_ = os.MkdirAll("cache", 0o755)
	oldCacheJSON := `{
  "bank_holidays": [
    "2020-01-01T00:00:00Z"
  ]
}`
	_ = os.WriteFile("cache/holidays_cache.json", []byte(oldCacheJSON), 0o644)

	mux := http.NewServeMux()
	mux.HandleFunc("/j_security_check", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session-newyear"})
		http.Redirect(w, r, "/home.xhtml", http.StatusFound)
	})

	calendarCalled := false
	mux.HandleFunc("/pages/employeePortal/calendarEmployee.xhtml", func(w http.ResponseWriter, r *http.Request) {
		calendarCalled = true
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		currentYear := time.Now().Year()
		fmt.Fprintf(w, `<div class="calendar-header panel panel-default">
<table><th class="year-title">%d</th></table>
</div>
<div class="month-container col-xs-2">
<table class="month">
<thead><tr><th class="month-title">Enero</th></tr></thead>
<tr><td class="day styled-day" style="background-color: rgb(238, 119, 68);"><div class="day-content">1</div></td></tr>
</table>
</div>`, currentYear)
	})

	mux.HandleFunc("/home.xhtml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, noHolidayHomeHTML())
			return
		}
		// POST click "Ver Calendario"
		w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<redirect url="/pages/employeePortal/calendarEmployee.xhtml"></redirect>
</partial-response>`)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	jar, _ := cookiejar.New(nil)
	clocker := clients.NewMyTeam2GoTestClocker(host, "u", "p", 0, 0, &http.Client{Jar: jar})

	clocker.IsHoliday(context.Background())
	assert.True(t, calendarCalled, "should fetch new year calendar when cache has previous year")
}

// TestMyTeam2Go_IsHoliday_BankHoliday_DataCalendarJSON models the current
// MyTeam2Go flow: the calendarEmployee.xhtml GET returns a JSF shell without any
// month grid. The browser fires $(document).ready → updateTabView (a PrimeFaces.ab
// call whose options include nested onst/onco functions), and that response carries
// the calendar events as a JSON array in <input id="dataCalendar" value="...">.
// Bank holidays are the events flagged isFestives=true.
func TestMyTeam2Go_IsHoliday_BankHoliday_DataCalendarJSON(t *testing.T) {
	_ = os.RemoveAll("cache")
	defer func() { _ = os.RemoveAll("cache") }()

	today := time.Now()
	bootHits := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/j_security_check", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "fake-session-json"})
		http.Redirect(w, r, "/home.xhtml", http.StatusFound)
	})

	mux.HandleFunc("/pages/employeePortal/calendarEmployee.xhtml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Shell: no month tables, no dataCalendar, just the boot call the
			// browser fires at load and the ViewState.
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			fmt.Fprint(w, `<!DOCTYPE html><html><body>
<form id="entityIdForm" name="entityIdForm" method="post" action="/pages/employeePortal/calendarEmployee.xhtml">
<input type="hidden" name="entityIdForm" value="entityIdForm"/>
<input type="hidden" name="jakarta.faces.ViewState" id="j_id1:jakarta.faces.ViewState:9" value="cal-vs-777" autocomplete="off"/>
</form>
<script>
var entityIdJs = ''; $(document).ready(function() { updateTabView(); });
updateTabView = function() {return PrimeFaces.ab({s:"entityIdForm:j_idt761",f:"entityIdForm",u:"tabView",onst:function(cfg){showLoading();},onco:function(xhr,status,args,data){hideLoading();;},pa:arguments[0]});}
</script>
</body></html>`)
			return
		}

		// POST — the boot call fired by the client
		bootHits++
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		assert.Equal(t, "true", r.FormValue("jakarta.faces.partial.ajax"))
		assert.Equal(t, "entityIdForm:j_idt761", r.FormValue("jakarta.faces.source"))
		assert.Equal(t, "cal-vs-777", r.FormValue("jakarta.faces.ViewState"))

		// Events: today (bank holiday), a holiday from a previous year (must be
		// ignored) and a personal vacation (isFestives=false, must be ignored).
		jsonEvents := fmt.Sprintf(`[
 {"isFestives":true,"typeAbsenceName":"Festivos nacionales","color":"#EE7744","dateAbsenceIni":"%[1]s %[2]d, %[3]d, 12:00:00 AM"},
 {"isFestives":true,"typeAbsenceName":"Festivos por localidad","color":"#AA7777","dateAbsenceIni":"Nov 10, %[4]d, 12:00:00 AM"},
 {"isFestives":false,"typeAbsenceName":"Vacaciones","color":"#116416","dateAbsenceIni":"%[1]s 20, %[3]d, 12:00:00 AM"}
]`, today.Month().String(), today.Day(), today.Year(), today.Year()-1)
		jsonEscaped := strings.NewReplacer(`"`, `&quot;`).Replace(jsonEvents)

		w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<changes>
<update id="tabView"><![CDATA[
<div id="tabView" class="ui-tabs">
<form id="tabView:calendarEmployeeForm"><input id="dataCalendar" type="hidden" value="%s"/></form>
</div>
]]></update>
<update id="j_id1:jakarta.faces.ViewState:9"><![CDATA[cal-vs-778]]></update>
</changes>
</partial-response>`, jsonEscaped)
	})

	mux.HandleFunc("/home.xhtml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, noHolidayHomeHTML())
			return
		}
		// POST click "Ver Calendario"
		w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<partial-response>
<redirect url="/pages/employeePortal/calendarEmployee.xhtml"></redirect>
</partial-response>`)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	jar, _ := cookiejar.New(nil)
	clocker := clients.NewMyTeam2GoTestClocker(host, "u", "p", 0, 0, &http.Client{Jar: jar})

	isHoliday := clocker.IsHoliday(context.Background())
	assert.True(t, isHoliday, "today should be detected as a bank holiday from the dataCalendar JSON")
	assert.Equal(t, 1, bootHits, "boot call (updateTabView) should be replayed exactly once")
}

func noHolidayHomeHTML() string {
	return `<!DOCTYPE html><html><body>
<input name="jakarta.faces.ViewState" value="home-vs-111">
<div id="topMenuIdForm:menuWork1"></div>
<a id="homeForm:j_idt1407" href="#" class="ui-commandlink ui-widget">Ver Calendario&gt;</a>
<script id="homePartialLoadings:j_idtCAL">
loadCalendaryWidget = function() {
    return PrimeFaces.ab({s:"homePartialLoadings:j_idtCAL", f:"homePartialLoadings",
                          u:"homeForm:calendarWidgetContent", a:true});
}
</script>
</body></html>`
}
