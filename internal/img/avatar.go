package img

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/scribe"
	"github.com/fogleman/gg"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	avatarSize         = 96
	avatarMaxBodyBytes = 2 * 1024 * 1024
	avatarSuccessTTL   = 8 * time.Hour
	avatarNegativeTTL  = 10 * time.Minute
	avatarFetchTimeout = 3 * time.Second
)

type avatarEntry struct {
	img       image.Image
	fetchedAt time.Time
	failed    bool
}

type AvatarCache struct {
	mu       sync.RWMutex
	entries  map[string]avatarEntry
	http     *http.Client
	scribe   *scribe.Client
	fallback image.Image
	logger   *log.Logger
}

func NewAvatarCache(s *scribe.Client) *AvatarCache {
	c := &AvatarCache{
		entries: make(map[string]avatarEntry),
		http:    &http.Client{Timeout: 5 * time.Second},
		scribe:  s,
		logger:  log.New(log.Default().Writer(), "[Avatar] ", log.Default().Flags()),
	}
	c.fallback = loadDefaultAvatar(c.logger)
	if c.fallback != nil {
		c.logger.Printf("loaded default avatar from ~/.mh-gobot/img/avatar_default.png")
	}
	return c
}

// Get returns a circle-cropped avatar at avatarSize. Never returns an error
// (failures fall through to the default placeholder, which may itself be nil
// if no placeholder file is present).
func (c *AvatarCache) Get(ctx context.Context, playFabID string) image.Image {
	c.mu.RLock()
	entry, ok := c.entries[playFabID]
	c.mu.RUnlock()
	if ok {
		ttl := avatarSuccessTTL
		if entry.failed {
			ttl = avatarNegativeTTL
		}
		if time.Since(entry.fetchedAt) < ttl {
			c.logger.Printf("cache hit for %s (age %s, failed=%v)", playFabID, time.Since(entry.fetchedAt).Round(time.Second), entry.failed)
			if entry.failed {
				return c.fallback
			}
			return entry.img
		}
	}

	img, err := c.fetch(ctx, playFabID)
	c.mu.Lock()
	if err != nil {
		c.logger.Printf("fetch failed for %s, negative-caching: %v", playFabID, err)
		c.entries[playFabID] = avatarEntry{fetchedAt: time.Now(), failed: true}
		c.mu.Unlock()
		return c.fallback
	}
	c.entries[playFabID] = avatarEntry{img: img, fetchedAt: time.Now()}
	c.mu.Unlock()
	return img
}

// GetMany fetches avatars for multiple players in parallel, each bounded by
// avatarFetchTimeout. Returns a map keyed by playFabID; failed entries map to
// the default placeholder (or nil if no placeholder is configured).
func (c *AvatarCache) GetMany(ctx context.Context, ids []string) map[string]image.Image {
	out := make(map[string]image.Image, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			fctx, cancel := context.WithTimeout(ctx, avatarFetchTimeout)
			defer cancel()
			img := c.Get(fctx, id)
			mu.Lock()
			out[id] = img
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return out
}

func (c *AvatarCache) fetch(ctx context.Context, playFabID string) (image.Image, error) {
	c.logger.Printf("fetching %s", playFabID)
	player, err := c.scribe.GetPlayer(ctx, playFabID)
	if err != nil {
		return nil, fmt.Errorf("scribe lookup: %w", err)
	}
	if player.AvatarURL == "" {
		return nil, errors.New("no avatar url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, player.AvatarURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "UltimateForm/ryard")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body := io.LimitReader(resp.Body, avatarMaxBodyBytes)
	img, _, err := image.Decode(body)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return circleCrop(resizeSquare(img, avatarSize)), nil
}

func loadDefaultAvatar(logger *log.Logger) image.Image {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".mh-gobot", "img", "avatar_default.png")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		logger.Printf("default avatar at %s failed to decode: %v", path, err)
		return nil
	}
	return circleCrop(resizeSquare(img, avatarSize))
}

// resizeSquare scales src into a size×size RGBA using a high-quality scaler.
func resizeSquare(src image.Image, size int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// circleCrop returns an RGBA image with src masked to a circle.
func circleCrop(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	mask := gg.NewContext(w, h)
	mask.DrawCircle(float64(w)/2, float64(h)/2, float64(w)/2)
	mask.Fill()
	dst := image.NewRGBA(b)
	draw.DrawMask(dst, b, src, image.Point{}, mask.Image(), image.Point{}, draw.Over)
	return dst
}
