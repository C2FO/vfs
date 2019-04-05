package backend

import (
	"sort"
	"sync"

	"github.com/c2fo/vfs/v3"
)

var mmu sync.RWMutex
var m map[string]vfs.FileSystem

// Register a new filesystem in backend map
func Register(name string, v vfs.FileSystem) {
	mmu.Lock()
	m[name] = v
	mmu.Unlock()
}

// Unregister unregisters a filesystem from backend map
func Unregister(name string) {
	mmu.Lock()
	delete(m, name)
	mmu.Unlock()
}

// UnregisterAll unregisters all filesystems from backend map
func UnregisterAll() {
	// mainly for tests
	mmu.Lock()
	m = make(map[string]vfs.FileSystem)
	mmu.Unlock()
}

// Backend returns the backend filesystem by name
func Backend(name string) vfs.FileSystem {
	mmu.RLock()
	defer mmu.RUnlock()
	return m[name]
}

// RegisteredBackends returns an array of backend names
func RegisteredBackends() []string {
	var f []string
	mmu.RLock()
	for k := range m {
		f = append(f, k)
	}
	mmu.RUnlock()
	sort.Strings(f)
	return f
}

func init() {
	m = make(map[string]vfs.FileSystem)
}
