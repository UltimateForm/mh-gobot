package img

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/util"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

const (
	leaderboardW         = 880.0
	leaderboardPadX      = 8.0
	leaderboardPadY      = 8.0
	leaderboardHeaderH   = 36.0
	leaderboardPodiumH   = 96.0
	leaderboardPodiumGap = 8.0
	leaderboardRowH      = 28.0
	leaderboardBaseFont  = 14.0
	leaderboardPodiumFnt = 20.0
	leaderboardRankFnt   = 22.0
)

type lbCol struct {
	label      string
	x          float64
	rightAlign bool
}

var lbCols = []lbCol{
	{"#", 0, true},
	{"PLAYER", 170, false},
	{"SCORE", 640, true},
	{"K", 720, true},
	{"D", 770, true},
	{"A", 820, true},
}

// RenderLeaderboardImage produces a PNG of the top-N leaderboard. The first
// three entries get podium treatment with their avatars (from the avatars
// map keyed by player_id). Missing avatars render as a plain rank chip.
// tiers maps player_id to the player's current rank tier; nil entries (or
// missing keys) render without a tier label.
func RenderLeaderboardImage(entries []data.RankedPlayer, avatars map[string]image.Image, tiers map[string]data.RankTier) (io.Reader, error) {
	podiumCount := min(len(entries), 3)
	standardCount := len(entries) - podiumCount

	podiumGapTotal := 0.0
	if podiumCount > 0 {
		podiumGapTotal = float64(podiumCount) * leaderboardPodiumGap
	}
	imgH := leaderboardPadY*2 + leaderboardHeaderH +
		float64(podiumCount)*leaderboardPodiumH + podiumGapTotal +
		float64(standardCount)*leaderboardRowH

	fnt, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}
	boldItalicFnt, err := truetype.Parse(fontBoldItalicBytes)
	if err != nil {
		return nil, err
	}
	baseFace := truetype.NewFace(fnt, &truetype.Options{Size: leaderboardBaseFont})
	podiumFace := truetype.NewFace(fnt, &truetype.Options{Size: leaderboardPodiumFnt})
	rankFace := truetype.NewFace(fnt, &truetype.Options{Size: leaderboardRankFnt})
	tierStandardFace := truetype.NewFace(boldItalicFnt, &truetype.Options{Size: 12.0})
	tierPodiumFace := truetype.NewFace(boldItalicFnt, &truetype.Options{Size: 14.0})
	nameStandardFace := truetype.NewFace(boldItalicFnt, &truetype.Options{Size: leaderboardBaseFont})
	namePodiumFace := truetype.NewFace(boldItalicFnt, &truetype.Options{Size: leaderboardPodiumFnt})

	dc := gg.NewContext(int(leaderboardW), int(imgH))
	dc.SetHexColor("#2B2D31")
	dc.Clear()

	drawHeader(dc, baseFace)

	cursorY := leaderboardPadY + leaderboardHeaderH

	for i := range podiumCount {
		cursorY += leaderboardPodiumGap / 2
		tier, hasTier := tiers[entries[i].PlayerID]
		drawPodiumRow(dc, cursorY, entries[i], avatars[entries[i].PlayerID], tier, hasTier, i, podiumFace, namePodiumFace, rankFace, tierPodiumFace)
		cursorY += leaderboardPodiumH + leaderboardPodiumGap/2
	}

	for i := podiumCount; i < len(entries); i++ {
		tier, hasTier := tiers[entries[i].PlayerID]
		drawStandardRow(dc, cursorY, entries[i], tier, hasTier, i-podiumCount, baseFace, nameStandardFace, tierStandardFace)
		cursorY += leaderboardRowH
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return &buf, nil
}

func drawHeader(dc *gg.Context, face font.Face) {
	dc.SetHexColor("#1E1F22")
	dc.DrawRectangle(0, leaderboardPadY, leaderboardW, leaderboardHeaderH)
	dc.Fill()
	dc.SetFontFace(face)
	dc.SetHexColor("#B5BAC1")
	for _, c := range lbCols {
		x := leaderboardPadX + c.x
		y := leaderboardPadY + (leaderboardHeaderH / 2) - 2
		if c.rightAlign {
			dc.DrawStringAnchored(c.label, x+28, y, 1, 0.5)
		} else {
			dc.DrawStringAnchored(c.label, x, y, 0, 0.5)
		}
	}
}

func drawPodiumRow(dc *gg.Context, rowY float64, e data.RankedPlayer, avatar image.Image, rank data.RankTier, hasRank bool, idx int, podiumFace, nameFace, rankFace, tierPodiumFace font.Face) {
	dc.SetHexColor("#26282D")
	dc.DrawRectangle(0, rowY, leaderboardW, leaderboardPodiumH)
	dc.Fill()

	chipColors := []string{"#FFD700", "#C0C0C0", "#CD7F32"}
	chipColor := chipColors[idx]

	chipR := 22.0
	chipCX := leaderboardPadX + 28.0
	chipCY := rowY + leaderboardPodiumH/2
	dc.SetHexColor(chipColor)
	dc.DrawCircle(chipCX, chipCY, chipR)
	dc.Fill()
	dc.SetFontFace(rankFace)
	dc.SetHexColor("#1E1F22")
	dc.DrawStringAnchored(fmt.Sprintf("%d", e.Rank), chipCX, chipCY-2, 0.5, 0.5)

	// avatar slot
	avatarSlotX := chipCX + chipR + 8
	avatarSlotY := rowY + (leaderboardPodiumH-avatarSize)/2
	if avatar != nil {
		dc.DrawImage(avatar, int(avatarSlotX), int(avatarSlotY))
	}

	textY := rowY + leaderboardPodiumH/2

	dc.SetFontFace(nameFace)
	dc.SetHexColor("#FFFFFF")
	playerCol := lbCols[1]
	playerX := leaderboardPadX + playerCol.x
	name := truncateToWidth(dc, e.Username, 480)
	dc.DrawStringAnchored(name, playerX, textY-8, 0, 0.5)

	if hasRank {
		tierName := rank.ShortName
		if tierName == "" {
			tierName = rank.Name
		}
		dc.SetFontFace(tierPodiumFace)
		dc.SetHexColor("#FFD700")
		dc.DrawStringAnchored(tierName, playerX, textY+14, 0, 0.5)
	}

	dc.SetFontFace(podiumFace)
	dc.SetHexColor("#FFD700")
	scoreCol := lbCols[2]
	dc.DrawStringAnchored(util.HumanFormat(e.Score), leaderboardPadX+scoreCol.x+28, textY, 1, 0.5)

	dc.SetHexColor("#DBDEE1")
	for _, pair := range []struct {
		col lbCol
		val int
	}{
		{lbCols[3], e.Kills},
		{lbCols[4], e.Deaths},
		{lbCols[5], e.Assists},
	} {
		dc.DrawStringAnchored(fmt.Sprintf("%d", pair.val), leaderboardPadX+pair.col.x+28, textY, 1, 0.5)
	}
}

func drawStandardRow(dc *gg.Context, rowY float64, e data.RankedPlayer, rank data.RankTier, hasRank bool, stripeIdx int, face, nameFace, tierStandardFace font.Face) {
	if stripeIdx%2 == 0 {
		dc.SetHexColor("#2B2D31")
	} else {
		dc.SetHexColor("#232428")
	}
	dc.DrawRectangle(0, rowY, leaderboardW, leaderboardRowH)
	dc.Fill()

	dc.SetFontFace(face)
	dc.SetHexColor("#DBDEE1")
	textY := rowY + (leaderboardRowH / 2) - 2
	vals := []string{
		fmt.Sprintf("%d", e.Rank),
		e.Username,
		util.HumanFormat(e.Score),
		fmt.Sprintf("%d", e.Kills),
		fmt.Sprintf("%d", e.Deaths),
		fmt.Sprintf("%d", e.Assists),
	}
	playerX := leaderboardPadX + lbCols[1].x
	var nameW float64
	for j, c := range lbCols {
		x := leaderboardPadX + c.x
		if c.rightAlign {
			dc.DrawStringAnchored(vals[j], x+28, textY, 1, 0.5)
		} else {
			dc.SetFontFace(nameFace)
			name := truncateToWidth(dc, vals[j], 350)
			if j == 1 {
				nameW, _ = dc.MeasureString(name)
			}
			dc.DrawStringAnchored(name, x, textY, 0, 0.5)
			dc.SetFontFace(face)
		}
	}

	if hasRank {
		tierName := rank.ShortName
		if tierName == "" {
			tierName = rank.Name
		}
		dc.SetFontFace(tierStandardFace)
		dc.SetHexColor("#B5BAC1")
		dc.DrawStringAnchored(tierName, playerX+nameW+8, textY, 0, 0.5)
	}
}

func truncateToWidth(dc *gg.Context, s string, maxW float64) string {
	for {
		w, _ := dc.MeasureString(s)
		if w <= maxW || len(s) == 0 {
			return s
		}
		s = s[:len(s)-1]
	}
}
