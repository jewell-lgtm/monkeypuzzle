package issuepicker

import (
	"sync"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

// IssueCache provides simple in-memory caching for issue search results.
type IssueCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	issues   []issue.IssueListItem
	cachedAt time.Time
}

// NewIssueCache creates a new cache with the specified TTL.
func NewIssueCache(ttl time.Duration) *IssueCache {
	return &IssueCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves cached issues for a query if not expired.
func (c *IssueCache) Get(query string) ([]issue.IssueListItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[query]
	if !ok || time.Since(entry.cachedAt) > c.ttl {
		return nil, false
	}
	return entry.issues, true
}

// Set stores issues for a query in the cache.
func (c *IssueCache) Set(query string, issues []issue.IssueListItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[query] = cacheEntry{issues: issues, cachedAt: time.Now()}
}

// Clear removes all entries from the cache.
func (c *IssueCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}
