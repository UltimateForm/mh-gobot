package img

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"io"
	"slices"

	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
)

//go:embed DejaVuSansMono.ttf
var fontBytes []byte

func RenderScoreboardImage(entries []*parse.ScoreboardEntry, skirmish bool) (io.Reader, error) {
	const (
		imgW       = 600.0
		fontSize   = 13.0
		lineHeight = 20.0
		padX       = 4.0
		padY       = 4.0
		headerH    = 28.0
	)

	type col struct {
		label      string
		x          float64
		rightAlign bool
	}

	var cols []col
	if skirmish {
		cols = []col{
			{"#", 0, true},
			{"PLAYER", 36, false},
			{"SCORE", 390, true},
			{"T", 430, true},
			{"K", 460, true},
			{"D", 490, true},
			{"A", 520, true},
		}
	} else {
		cols = []col{
			{"#", 0, true},
			{"PLAYER", 36, false},
			{"SCORE", 420, true},
			{"K", 460, true},
			{"D", 490, true},
			{"A", 520, true},
		}
	}

	sorted := make([]*parse.ScoreboardEntry, len(entries))
	copy(sorted, entries)
	slices.SortFunc(sorted, func(a, b *parse.ScoreboardEntry) int { return b.Score - a.Score })

	imgH := padY*2 + headerH + float64(len(sorted))*lineHeight

	fnt, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}

	dc := gg.NewContext(int(imgW), int(imgH))
	dc.SetHexColor("#2B2D31")
	dc.Clear()

	face := truetype.NewFace(fnt, &truetype.Options{Size: fontSize})
	dc.SetFontFace(face)

	// header
	dc.SetHexColor("#1E1F22")
	dc.DrawRectangle(0, padY, imgW, headerH)
	dc.Fill()
	dc.SetHexColor("#B5BAC1")
	for _, c := range cols {
		x := padX + c.x
		// need to add -2 due to font face skew
		y := padY + (headerH / 2) - 2
		if c.rightAlign {
			dc.DrawStringAnchored(c.label, x+28, y, 1, 0.5)
		} else {
			dc.DrawStringAnchored(c.label, x, y, 0, 0.5)
		}
	}

	for i, e := range sorted {
		y := padY + headerH + float64(i)*lineHeight
		if i%2 == 0 {
			dc.SetHexColor("#2B2D31")
		} else {
			dc.SetHexColor("#232428")
		}
		dc.DrawRectangle(0, y, imgW, lineHeight)
		dc.Fill()

		if skirmish {
			if e.TeamID == 1 {
				dc.SetHexColor("#c24141")
			} else {
				dc.SetHexColor("#4b85d8")
			}
			dc.DrawRectangle(0, y, 3, lineHeight)
			dc.Fill()
		}

		dc.SetHexColor("#DBDEE1")
		textY := y + (lineHeight / 2) - 2
		var vals []string
		if skirmish {
			vals = []string{
				fmt.Sprintf("%d", i+1),
				e.UserName,
				fmt.Sprintf("%d", e.Score),
				fmt.Sprintf("%d", e.TeamID),
				fmt.Sprintf("%d", e.Kills),
				fmt.Sprintf("%d", e.Deaths),
				fmt.Sprintf("%d", e.Assists),
			}
		} else {
			vals = []string{
				fmt.Sprintf("%d", i+1),
				e.UserName,
				fmt.Sprintf("%d", e.Score),
				fmt.Sprintf("%d", e.Kills),
				fmt.Sprintf("%d", e.Deaths),
				fmt.Sprintf("%d", e.Assists),
			}
		}
		for j, c := range cols {
			x := padX + c.x
			if c.rightAlign {
				dc.DrawStringAnchored(vals[j], x+28, textY, 1, 0.5)
			} else {
				name := vals[j]
				for {
					w, _ := dc.MeasureString(name)
					if w <= 340 {
						break
					}
					name = name[:len(name)-1]
				}
				dc.DrawStringAnchored(name, x, textY, 0, 0.5)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return &buf, nil
}
