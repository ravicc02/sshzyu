package timezone

import (
	"sync"
	"time"
)

type locationResult struct {
	once     sync.Once
	location *time.Location
	err      error
}

var locationCache sync.Map

// LoadLocation parses an IANA zone file once per process. Invalid names are
// not retained; Local stays dynamic to honor Init and caller time.Local changes.
func LoadLocation(name string) (*time.Location, error) {
	if name == "Local" {
		return time.Local, nil
	}
	value, _ := locationCache.LoadOrStore(name, &locationResult{})
	entry := value.(*locationResult)
	entry.once.Do(func() { entry.location, entry.err = time.LoadLocation(name) })
	if entry.err != nil {
		locationCache.Delete(name)
	}
	return entry.location, entry.err
}
