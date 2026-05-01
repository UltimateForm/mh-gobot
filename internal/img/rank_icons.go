package img

import (
	"bytes"
	"context"
	"image"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/UltimateForm/mh-gobot/internal/data"
)

type RankIconCache struct {
	mu     sync.RWMutex
	icons  map[string]image.Image
	logger *log.Logger
}

func NewRankIconCache() *RankIconCache {
	c := &RankIconCache{
		icons:  make(map[string]image.Image),
		logger: log.New(log.Default().Writer(), "[RankIcons] ", log.Default().Flags()),
	}
	c.loadIcons()
	return c
}

func (c *RankIconCache) loadIcons() {
	ctx := context.Background()
	tiers, err := data.ReadRankTiers(ctx)
	if err != nil {
		c.logger.Printf("failed to read rank tiers: %v", err)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		c.logger.Printf("failed to get home dir: %v", err)
		return
	}
	imgDir := filepath.Join(home, ".mh-gobot", "img")

	loaded := 0
	for _, tier := range tiers {
		filename := filenameFromRankName(tier.Name)

		var iconPath string
		for _, ext := range []string{".webp", ".png", ".jpg"} {
			candidate := filepath.Join(imgDir, filename+ext)
			if _, err := os.Stat(candidate); err == nil {
				iconPath = candidate
				break
			}
		}

		if iconPath == "" {
			c.logger.Printf("no icon file found for rank %s (tried %s.*)", tier.Name, filename)
			continue
		}

		fileData, err := os.ReadFile(iconPath)
		if err != nil {
			c.logger.Printf("failed to read %s: %v", iconPath, err)
			continue
		}

		img, _, err := image.Decode(bytes.NewReader(fileData))
		if err != nil {
			c.logger.Printf("failed to decode %s: %v", filepath.Base(iconPath), err)
			continue
		}

		c.icons[tier.Name] = img
		loaded++
	}
	c.logger.Printf("loaded %d rank icons", loaded)
}

func filenameFromRankName(rankName string) string {
	return strings.ToLower(strings.ReplaceAll(rankName, " ", "_"))
}

func (c *RankIconCache) Get(rankName string) image.Image {
	c.mu.RLock()
	// unsure we need readlock here at all claude but fine ill trust you
	defer c.mu.RUnlock()
	return c.icons[rankName]
}

func (c *RankIconCache) All() map[string]image.Image {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]image.Image, len(c.icons))
	for k, v := range c.icons {
		out[k] = v
	}
	return out
}
