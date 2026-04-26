package scribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	baseURL    = "https://api.mordhau-scribe.com:8443"
	userAgent  = "UltimateForm/ryard"
	defaultTTL = 8 * time.Hour
)

var ErrPlayerNotFound = errors.New("scribe: player not found")

type ScribePlayer struct {
	PlayFabID string `json:"playFabId"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type cacheEntry struct {
	player    ScribePlayer
	fetchedAt time.Time
}

type Client struct {
	http   *http.Client
	cache  map[string]cacheEntry
	mu     sync.RWMutex
	ttl    time.Duration
	logger *log.Logger
}

func NewClient() *Client {
	return &Client{
		http:   &http.Client{Timeout: 10 * time.Second},
		cache:  make(map[string]cacheEntry),
		ttl:    defaultTTL,
		logger: log.New(log.Default().Writer(), "[Scribe] ", log.Default().Flags()),
	}
}

func (c *Client) GetPlayer(ctx context.Context, playFabID string) (*ScribePlayer, error) {
	// fyi about ctx, the client has a static 10 secs timeout already
	c.mu.RLock()
	entry, ok := c.cache[playFabID]
	c.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		c.logger.Printf("cache hit for %s (age %s)", playFabID, time.Since(entry.fetchedAt).Round(time.Second))
		p := entry.player
		return &p, nil
	}

	c.logger.Printf("fetching %s", playFabID)
	url := fmt.Sprintf("%s/api/players/%s", baseURL, playFabID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Printf("fetch error for %s: %v", playFabID, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPlayerNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.Printf("unexpected status %d for %s", resp.StatusCode, playFabID)
		return nil, fmt.Errorf("scribe: unexpected status %d", resp.StatusCode)
	}

	var player ScribePlayer
	if err := json.NewDecoder(resp.Body).Decode(&player); err != nil {
		c.logger.Printf("decode error for %s: %v", playFabID, err)
		return nil, err
	}

	c.mu.Lock()
	c.cache[playFabID] = cacheEntry{player: player, fetchedAt: time.Now()}
	c.mu.Unlock()

	return &player, nil
}
