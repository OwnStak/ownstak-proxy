package server

import (
	"hash/fnv"
	"sync"
	"time"
)

// CacheEntry represents a single entry in the cache
type CacheEntry struct {
	Value      string
	Expiration time.Time
	Size       int // Size in bytes
}

// CacheSection represents a single section of the cache
// Each section has its own lock to enable concurrent access to different sections
type CacheSection struct {
	mu          sync.RWMutex
	entries     map[string]CacheEntry
	maxSize     int // Maximum size in bytes for this section
	currentSize int // Current total size in bytes for this section
}

// Cache represents a thread-safe cache with expiration support
// The cache is split into multiple sections to reduce lock contention under high load
type Cache struct {
	sections     []*CacheSection
	sectionCount int
	maxSize      int // Total maximum size in bytes
}

// NewCache creates a new sectioned cache with the specified maximum size in bytes
func NewCache(maxSize int) *Cache {
	// Use 16 sections by default - this reduces lock contention for high concurrency
	sectionCount := 16

	// Create the cache with multiple sections
	cache := &Cache{
		sections:     make([]*CacheSection, sectionCount),
		sectionCount: sectionCount,
		maxSize:      maxSize,
	}

	// Calculate max size per section (evenly distributed)
	sectionMaxSize := maxSize / sectionCount

	// Initialize each section
	for i := 0; i < sectionCount; i++ {
		cache.sections[i] = &CacheSection{
			entries: make(map[string]CacheEntry),
			maxSize: sectionMaxSize,
		}
	}

	// Start a background cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			cache.CleanupExpired()
		}
	}()

	return cache
}

// getSection returns the appropriate section for a given key
// This distributes keys across sections based on their hash
func (c *Cache) getSection(key string) *CacheSection {
	// Use FNV hash for string keys - fast and good distribution
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := h.Sum32()

	// Map hash to section index
	sectionIndex := hash % uint32(c.sectionCount)
	return c.sections[sectionIndex]
}

// Set adds a key-value pair to the cache with an optional expiration time
// Returns true if the value was set, false if it couldn't be set due to size constraints
func (c *Cache) Set(key string, value string, expiration time.Duration) bool {
	// Get the appropriate section for this key
	section := c.getSection(key)

	// Lock only this section - other sections remain accessible
	section.mu.Lock()
	defer section.mu.Unlock()

	// Calculate the size of the new entry in bytes
	newSize := len(key) + len(value)

	// If this is an update, subtract the size of the existing entry
	if oldEntry, exists := section.entries[key]; exists {
		section.currentSize -= oldEntry.Size
	}

	// Check if adding this entry would exceed the maximum size for this section
	if section.currentSize+newSize > section.maxSize {
		// Try to make room by removing expired entries in this section
		removeExpiredEntriesFromSection(section)

		// Check again after clearing expired entries
		if section.currentSize+newSize > section.maxSize {
			return false // Cannot set the value, would exceed maximum size
		}
	}

	// Calculate expiration time
	var expirationTime time.Time
	if expiration > 0 {
		expirationTime = time.Now().Add(expiration)
	} else {
		expirationTime = time.Now().Add(time.Minute * 5) // Defaults to 5 minutes if no expiration is provided
	}

	// Add the entry to the cache section
	section.entries[key] = CacheEntry{
		Value:      value,
		Expiration: expirationTime,
		Size:       newSize,
	}
	section.currentSize += newSize

	return true
}

// Get retrieves a value from the cache by key
// Returns the value and a boolean indicating if the value was found and not expired
func (c *Cache) Get(key string) (string, bool) {
	// Get the appropriate section for this key
	section := c.getSection(key)

	// Use a read lock when getting values
	section.mu.RLock()
	entry, exists := section.entries[key]

	// Check if the entry has expired - do this under read lock
	isExpired := exists && !entry.Expiration.IsZero() && time.Now().After(entry.Expiration)
	section.mu.RUnlock()

	if !exists {
		return "", false
	}

	// Handle expired entries without creating goroutines
	if isExpired {
		// Delete expired entry under a write lock
		section.mu.Lock()
		delete(section.entries, key)
		section.currentSize -= entry.Size
		section.mu.Unlock()
		return "", false
	}

	return entry.Value, true
}

// Delete removes a key-value pair from the cache
func (c *Cache) Delete(key string) {
	// Get the appropriate section for this key
	section := c.getSection(key)

	section.mu.Lock()
	defer section.mu.Unlock()

	if entry, exists := section.entries[key]; exists {
		section.currentSize -= entry.Size
		delete(section.entries, key)
	}
}

// Size returns the current size of the cache in bytes (across all sections)
func (c *Cache) Size() int {
	totalSize := 0

	// Sum up the size of each section
	for i := 0; i < c.sectionCount; i++ {
		section := c.sections[i]
		section.mu.RLock()
		totalSize += section.currentSize
		section.mu.RUnlock()
	}

	return totalSize
}

// Count returns the number of entries in the cache (across all sections)
func (c *Cache) Count() int {
	totalCount := 0

	// Sum up the count of each section
	for i := 0; i < c.sectionCount; i++ {
		section := c.sections[i]
		section.mu.RLock()
		totalCount += len(section.entries)
		section.mu.RUnlock()
	}

	return totalCount
}

// Clear removes all entries from the cache (clearing all sections)
func (c *Cache) Clear() {
	for i := 0; i < c.sectionCount; i++ {
		section := c.sections[i]
		section.mu.Lock()
		section.entries = make(map[string]CacheEntry)
		section.currentSize = 0
		section.mu.Unlock()
	}
}

// removeExpiredEntriesFromSection removes all expired entries from a specific section
// Note: This function assumes the section's mutex is already locked
func removeExpiredEntriesFromSection(section *CacheSection) {
	now := time.Now()
	for key, entry := range section.entries {
		if !entry.Expiration.IsZero() && now.After(entry.Expiration) {
			section.currentSize -= entry.Size
			delete(section.entries, key)
		}
	}
}

// CleanupExpired removes all expired entries from the cache
func (c *Cache) CleanupExpired() {
	now := time.Now()

	for i := 0; i < c.sectionCount; i++ {
		section := c.sections[i]
		section.mu.Lock()

		for key, entry := range section.entries {
			if !entry.Expiration.IsZero() && now.After(entry.Expiration) {
				section.currentSize -= entry.Size
				delete(section.entries, key)
			}
		}

		section.mu.Unlock()
	}
}
