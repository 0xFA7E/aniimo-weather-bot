package vision

import (
	"fmt"
	"image"
	"image/color"
)

// Classification deliberately identifies only a candidate. Extraction must
// validate text and weather rows before an observation can produce an alert.
type Classification struct {
	Candidate        bool    `json:"candidate"`
	BodyBrightRatio  float64 `json:"body_bright_ratio"`
	BodyNeutralRatio float64 `json:"body_neutral_ratio"`
	HeaderBlueRatio  float64 `json:"header_blue_ratio"`
}

func Classify(img image.Image) Classification {
	b := img.Bounds()
	body := normalizedRect(b, 0.76, 0.22, 0.985, 0.90)
	header := normalizedRect(b, 0.76, 0.02, 0.985, 0.21)

	var bodyN, bright, neutral int
	for y := body.Min.Y; y < body.Max.Y; y += 2 {
		for x := body.Min.X; x < body.Max.X; x += 2 {
			r, g, bl := rgb8(img.At(x, y))
			bodyN++
			mean := (int(r) + int(g) + int(bl)) / 3
			if mean > 170 {
				bright++
			}
			if mean > 130 && int(max3(r, g, bl))-int(min3(r, g, bl)) < 28 {
				neutral++
			}
		}
	}

	var headerN, blue int
	for y := header.Min.Y; y < header.Max.Y; y += 2 {
		for x := header.Min.X; x < header.Max.X; x += 2 {
			r, g, bl := rgb8(img.At(x, y))
			headerN++
			if bl > 150 && int(bl) > int(r)+18 && int(g) > int(r)+8 {
				blue++
			}
		}
	}

	c := Classification{
		BodyBrightRatio:  ratio(bright, bodyN),
		BodyNeutralRatio: ratio(neutral, bodyN),
		HeaderBlueRatio:  ratio(blue, headerN),
	}
	c.Candidate = c.BodyBrightRatio >= 0.78 &&
		c.BodyNeutralRatio >= 0.70 && c.HeaderBlueRatio >= 0.16
	return c
}

func (c Classification) Reason() string {
	return fmt.Sprintf("forecast-card candidate=%t bright=%.3f neutral=%.3f headerBlue=%.3f",
		c.Candidate, c.BodyBrightRatio, c.BodyNeutralRatio, c.HeaderBlueRatio)
}

func normalizedRect(b image.Rectangle, x0, y0, x1, y1 float64) image.Rectangle {
	w, h := b.Dx(), b.Dy()
	return image.Rect(
		b.Min.X+int(float64(w)*x0), b.Min.Y+int(float64(h)*y0),
		b.Min.X+int(float64(w)*x1), b.Min.Y+int(float64(h)*y1),
	)
}

func rgb8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func max3(a, b, c uint8) uint8 {
	if a < b {
		a = b
	}
	if a < c {
		a = c
	}
	return a
}

func min3(a, b, c uint8) uint8 {
	if a > b {
		a = b
	}
	if a > c {
		a = c
	}
	return a
}
