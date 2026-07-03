package clockers

import (
	"fmt"
	"math/rand/v2"
)

// humanLocation returns location strings to include in the clock-in form.
//
// If coordinates are configured, it returns a slightly jittered version
// simulating the natural imprecision of a real browser's Geolocation API
// (±~11 m random offset, 6 decimal places, realistic accuracy in metres).
//
// If no coordinates are configured (both zero), it returns empty lat/lon
// and the permission-denied error message that Chrome sends when the user
// blocks geolocation access — indistinguishable from a real denial.
func humanLocation(latitude, longitude float64) (lat, lon, locationErr string) {
	if latitude == 0 && longitude == 0 {
		// Mimic MyTeam2Go's permission-denied payload. Could be different with other clockers.
		return "", "", "geolocation.error.permission_denied"
	}

	// ±0.0001° ≈ ±11 m, well within normal GPS/WiFi-positioning variance.
	jitter := func() float64 { return (rand.Float64()*2 - 1) * 0.0001 }
	lat = fmt.Sprintf("%.14g", latitude+jitter())
	lon = fmt.Sprintf("%.14g", longitude+jitter())
	// locationError is left empty when successful
	locationErr = ""
	return
}
