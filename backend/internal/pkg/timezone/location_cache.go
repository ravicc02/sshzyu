package timezone

import (
	"sync"
	"time"
)

var cachedLocations = struct {
	sync.RWMutex
	locations map[string]*time.Location
}{locations: make(map[string]*time.Location)}

// LoadLocation reuses parsed IANA timezones on quota and billing hot paths.
// Local remains dynamic; failed lookups are not retained.
func LoadLocation(name string) (*time.Location, error) {
	if name == "" || name == "UTC" || name == "Local" {
		return time.LoadLocation(name)
	}
	cachedLocations.RLock()
	loc := cachedLocations.locations[name]
	cachedLocations.RUnlock()
	if loc != nil {
		return loc, nil
	}
	cachedLocations.Lock()
	defer cachedLocations.Unlock()
	if loc := cachedLocations.locations[name]; loc != nil {
		return loc, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	cachedLocations.locations[name] = loc
	return loc, nil
}
