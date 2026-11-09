package clients

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCalendarEmployeeHTML(t *testing.T) {
	// Simulate 12 months with different/arbitrary language names to test language independence
	monthTitles := []string{"Month 1", "Month 2", "Month 3", "Month 4", "Mayo (or May / Maig)", "Month 6", "Month 7", "Month 8", "Month 9", "Month 10", "Month 11", "Month 12"}

	var tablesHTML string
	for i, title := range monthTitles {
		if i == 4 { // Month 5 (Mayo)
			tablesHTML += fmt.Sprintf(`<div class="month-container col-xs-2">
<table class="month">
<thead>
<tr><th class="month-title" colspan="7">%s</th></tr>
<tr><th class="day-header">Lu</th><th class="day-header">Ma</th><th class="day-header">Mi</th><th class="day-header">Ju</th><th class="day-header">Vi</th><th class="day-header">Sa</th><th class="day-header">Do</th></tr>
</thead>
<tr>
<td class="day old"></td><td class="day old"></td><td class="day old"></td><td class="day old"></td>
<td class="day styled-day" style="background-color: rgb(238, 119, 68); color: white; border-color: rgb(238, 119, 68);" data-original-title="" title=""><div class="day-content" style="color: rgb(255, 255, 255) !important;">1</div></td>
<td class="day styled-day" style="background-color: rgb(210, 47, 50); color: white; border-color: rgb(210, 47, 50);" data-original-title="" title=""><div class="day-content" style="color: rgb(255, 255, 255) !important;">2</div></td>
<td class="day"><div class="day-content">3</div></td>
</tr>
<tr>
<td class="day"><div class="day-content">4</div></td>
<td class="day"><div class="day-content">5</div></td>
<td class="day"><div class="day-content">6</div></td>
<td class="day"><div class="day-content">7</div></td>
<td class="day"><div class="day-content">8</div></td>
<td class="day"><div class="day-content">9</div></td>
<td class="day"><div class="day-content">10</div></td>
</tr>
<tr>
<td class="day styled-day" style="background-color: rgb(17, 100, 22); color: white;" title=""><div class="day-content">11</div></td>
<td class="day"><div class="day-content">12</div></td>
<td class="day"><div class="day-content">13</div></td>
<td class="day"><div class="day-content">14</div></td>
<td class="day styled-day" style="background-color: rgb(170, 119, 119); color: white; border-color: rgb(170, 119, 119);" data-original-title="" title=""><div class="day-content" style="color: rgb(255, 255, 255) !important;">15</div></td>
<td class="day"><div class="day-content">16</div></td>
<td class="day"><div class="day-content">17</div></td>
</tr>
</table>
</div>`, title)
		} else {
			tablesHTML += fmt.Sprintf(`<div class="month-container col-xs-2">
<table class="month">
<thead><tr><th class="month-title">%s</th></tr></thead>
<tr><td class="day"><div class="day-content">1</div></td></tr>
</table>
</div>`, title)
		}
	}

	html := fmt.Sprintf(`<div class="calendar-header panel panel-default">
<table>
<th class="prev"><span class="glyphicon glyphicon-chevron-left"></span></th>
<th class="year-title year-neighbor hidden-xs">2025</th>
<th class="year-title">2026</th>
<th class="year-title year-neighbor hidden-xs disabled">2027</th>
<th class="next disabled"><span class="glyphicon glyphicon-chevron-right"></span></th>
</table>
</div>
%s`, tablesHTML)

	bankHolidays, err := parseCalendarEmployeeHTML(html, 2026)
	require.NoError(t, err)

	// Month 5 (Mayo) 1 (national: 238, 119, 68), Mayo 2 (community: 210, 47, 50), Mayo 15 (city: 170, 119, 119)
	// Mayo 11 is requested vacation (17, 100, 22) and must be excluded from bank_holidays
	expectedMay1 := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.Now().Location())
	expectedMay2 := time.Date(2026, time.May, 2, 0, 0, 0, 0, time.Now().Location())
	expectedMay15 := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.Now().Location())
	expectedMay11 := time.Date(2026, time.May, 11, 0, 0, 0, 0, time.Now().Location())

	assert.Contains(t, bankHolidays, expectedMay1)
	assert.Contains(t, bankHolidays, expectedMay2)
	assert.Contains(t, bankHolidays, expectedMay15)
	assert.NotContains(t, bankHolidays, expectedMay11)
	assert.Len(t, bankHolidays, 3)
}

func TestHolidayCache_BankHolidaysHelpers(t *testing.T) {
	cache := holidayCache{
		BankHolidays: []time.Time{
			time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		},
	}

	assert.True(t, hasBankHolidaysForYear(cache, 2026))
	assert.False(t, hasBankHolidaysForYear(cache, 2027))

	d1 := time.Date(2026, time.May, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, time.May, 2, 10, 0, 0, 0, time.UTC)
	assert.True(t, isBankHolidayInCache(cache, d1))
	assert.False(t, isBankHolidayInCache(cache, d2))
}
