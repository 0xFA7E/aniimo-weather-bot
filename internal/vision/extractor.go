package vision

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	timeTokenRE = regexp.MustCompile(`(?i)\b(\d{1,2}:\d{2})\s*(AM|PM)\b`)
	clockRE     = regexp.MustCompile(`(?i)\b\d{1,2}:\d{2}(?::\d{2})?\s*[,.]?\s*(?:AM|PM)\b`)
	utcRE       = regexp.MustCompile(`(?i)\bUTC\s*([+-])\s*(\d{1,2})\b`)
)

var knownRegions = []string{
	"The Argent Strait", "Echoback Landing", "Berylline Vale", "Russet Highlands",
	"Beast Fang Ridge", "Driftwise Meadow", "Mistwoods", "Forest of Falling Stars",
	"Nimbus Field", "Sea of Flowers", "Blitzwood", "Tideblossom Coast",
	"Rosetower Woods", "Zephyrus Landbridge",
}

func IsKnownRegion(region string) bool {
	for _, known := range knownRegions {
		if region == known {
			return true
		}
	}
	return false
}

type ForecastRow struct {
	Day         string  `json:"day"`
	Start       string  `json:"start"`
	End         string  `json:"end"`
	Weather     string  `json:"weather"`
	YellowRatio float64 `json:"yellow_ratio"`
	BlueRatio   float64 `json:"blue_ratio"`
}

type Extraction struct {
	Valid       bool          `json:"valid"`
	Region      string        `json:"region,omitempty"`
	Timezone    string        `json:"timezone,omitempty"`
	CurrentTime string        `json:"current_time,omitempty"`
	Rows        []ForecastRow `json:"rows,omitempty"`
	Reason      string        `json:"reason"`
}

type word struct {
	text       string
	x, y, w, h int
	block      int
	paragraph  int
	line       int
}

// Extract invokes the locally installed Tesseract executable. A screenshot is
// valid only when both the panel heading and a useful set of forecast rows are
// independently recognized.
func Extract(ctx context.Context, img image.Image, tesseract string) (Extraction, error) {
	var err error
	tesseract, err = resolveTesseract(tesseract)
	if err != nil {
		return Extraction{}, err
	}

	words, err := runTSV(ctx, tesseract, img, 11)
	if err != nil {
		return Extraction{}, err
	}
	lines := groupLines(words)
	allText := strings.ToLower(strings.Join(lineTexts(lines), " "))
	anchor := strings.Contains(allText, "special weather") && strings.Contains(allText, "next 24 hours")

	forecast, forecastOffsetY := cropForecast(img)
	forecastWords, forecastErr := runTSV(ctx, tesseract, forecast, 6)
	var rows []ForecastRow
	if forecastErr == nil {
		rows = parseRows(img, groupLines(forecastWords), forecastOffsetY)
	}
	result := Extraction{Rows: rows}
	if !anchor {
		result.Reason = "forecast heading not recognized"
		return result, nil
	}
	if len(rows) < 4 {
		result.Reason = fmt.Sprintf("forecast heading recognized but only %d time rows validated", len(rows))
		return result, nil
	}

	rawHeader := cropHeader(img)
	rawWords, rawErr := runTSV(ctx, tesseract, rawHeader, 6)
	var regionCandidates []string
	if rawErr == nil {
		rawLines := groupLines(rawWords)
		parseHeader(rawLines, &result)
		regionCandidates = append(regionCandidates, headerRegionCandidates(rawLines)...)
	}
	header := preprocessHeader(rawHeader)
	headerWords, headerErr := runTSV(ctx, tesseract, header, 6)
	if headerErr == nil {
		enhancedLines := groupLines(headerWords)
		parseHeader(enhancedLines, &result)
		regionCandidates = append(regionCandidates, headerRegionCandidates(enhancedLines)...)
	}
	for _, threshold := range []uint8{166, 191, 204} {
		regionImage := preprocessRegion(img, threshold)
		if regionWords, regionErr := runTSV(ctx, tesseract, regionImage, 7); regionErr == nil {
			regionCandidates = append(regionCandidates, headerRegionCandidates(groupLines(regionWords))...)
		}
	}
	if result.Region != "" {
		regionCandidates = append(regionCandidates, result.Region)
	}
	result.Region = matchKnownRegion(regionCandidates)
	result.Valid = true
	result.Reason = fmt.Sprintf("validated forecast heading and %d time rows", len(rows))
	return result, nil
}

func resolveTesseract(configured string) (string, error) {
	executable, _ := os.Executable()
	return resolveTesseractAt(configured, executable, runtime.GOOS, exec.LookPath)
}

func resolveTesseractAt(configured, applicationExecutable, goos string, lookPath func(string) (string, error)) (string, error) {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured, nil
	}
	if applicationExecutable != "" {
		name := "tesseract"
		if goos == "windows" {
			name += ".exe"
		}
		bundled := filepath.Join(filepath.Dir(applicationExecutable), "tesseract", name)
		if info, statErr := os.Stat(bundled); statErr == nil && !info.IsDir() {
			return bundled, nil
		}
	}
	found, err := lookPath("tesseract")
	if err != nil {
		return "", fmt.Errorf("find tesseract: bundled OCR runtime is missing and tesseract is not on PATH: %w", err)
	}
	return found, nil
}

func runTSV(ctx context.Context, executable string, img image.Image, psm int) ([]word, error) {
	var input bytes.Buffer
	if err := png.Encode(&input, img); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, "stdin", "stdout", "-l", "eng", "--psm", strconv.Itoa(psm), "tsv")
	cmd.Stdin = &input
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var result []word
	for i, raw := range strings.Split(stdout.String(), "\n") {
		r := strings.SplitN(strings.TrimSuffix(raw, "\r"), "\t", 12)
		if i == 0 || len(r) < 12 || strings.TrimSpace(r[11]) == "" {
			continue
		}
		ints := make([]int, 9)
		ok := true
		for j := 1; j <= 9; j++ {
			value, parseErr := strconv.Atoi(r[j])
			if parseErr != nil {
				ok = false
				break
			}
			ints[j-1] = value
		}
		if ok {
			result = append(result, word{text: r[11], block: ints[1], paragraph: ints[2], line: ints[3], x: ints[5], y: ints[6], w: ints[7], h: ints[8]})
		}
	}
	return result, nil
}

func groupLines(words []word) [][]word {
	type key struct{ block, paragraph, line int }
	order := make([]key, 0)
	groups := make(map[key][]word)
	for _, w := range words {
		k := key{w.block, w.paragraph, w.line}
		if _, exists := groups[k]; !exists {
			order = append(order, k)
		}
		groups[k] = append(groups[k], w)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := groups[order[i]][0], groups[order[j]][0]
		if a.y != b.y {
			return a.y < b.y
		}
		return a.x < b.x
	})
	result := make([][]word, 0, len(order))
	for _, k := range order {
		sort.Slice(groups[k], func(i, j int) bool { return groups[k][i].x < groups[k][j].x })
		result = append(result, groups[k])
	}
	return result
}

func lineTexts(lines [][]word) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := make([]string, len(line))
		for i, w := range line {
			parts[i] = w.text
		}
		result = append(result, strings.Join(parts, " "))
	}
	return result
}

func parseRows(img image.Image, lines [][]word, yOffset int) []ForecastRow {
	day := ""
	var rows []ForecastRow
	for _, line := range lines {
		text := strings.Join(wordTexts(line), " ")
		lower := strings.ToLower(text)
		if strings.Contains(lower, "today") {
			day = "today"
		}
		if strings.Contains(lower, "tomorrow") {
			day = "tomorrow"
		}
		if !strings.ContainsAny(text, "-–—") {
			continue
		}
		tokens := timeTokenRE.FindAllStringSubmatch(text, -1)
		if len(tokens) == 0 {
			continue
		}
		endToken := tokens[len(tokens)-1]
		end, start, ok := forecastInterval(endToken[1], endToken[2])
		if !ok {
			continue
		}
		y := lineCenter(line) + yOffset
		yellow, blue := iconRatios(img, y)
		weather := classifyWeather(yellow, blue)
		rows = append(rows, ForecastRow{
			Day: day, Start: start, End: end,
			Weather: weather, YellowRatio: yellow, BlueRatio: blue,
		})
	}
	return rows
}

func forecastInterval(endValue, meridiem string) (end string, start string, ok bool) {
	parsed, err := time.Parse("03:04 PM", endValue+" "+strings.ToUpper(meridiem))
	if err != nil || parsed.Minute() != 59 {
		return "", "", false
	}
	hour := parsed.Hour()
	if hour != 0 && hour != 4 && hour != 8 && hour != 12 && hour != 16 && hour != 20 {
		return "", "", false
	}
	return parsed.Format("03:04 PM"), parsed.Add(-3*time.Hour - 59*time.Minute).Format("03:04 PM"), true
}

func cropForecast(src image.Image) (image.Image, int) {
	b := src.Bounds()
	r := normalizedRect(b, .7656, .4167, .9219, .8426)
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			dst.Set(x, y, src.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return dst, r.Min.Y
}

func parseHeader(lines [][]word, result *Extraction) {
	for _, line := range lines {
		text := strings.Join(wordTexts(line), " ")
		if m := utcRE.FindStringSubmatch(text); m != nil {
			result.Timezone = "UTC" + m[1] + m[2]
		}
		if m := clockRE.FindStringIndex(text); m != nil {
			result.CurrentTime = strings.ReplaceAll(strings.TrimSpace(text[m[0]:m[1]]), ",", "")
			candidate := cleanRegion(text[:m[0]])
			if len(candidate) >= 3 {
				result.Region = candidate
			}
		}
	}
}

func cleanRegion(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"Sunny", "Cloudy", "Rainy", "Rain", "Snowy", "Snow"} {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
			s = strings.TrimSpace(s[len(prefix):])
		}
	}
	s = strings.Trim(s, " |.,:;_-–—")
	return strings.Join(strings.Fields(s), " ")
}

func headerRegionCandidates(lines [][]word) []string {
	var result []string
	for _, line := range lines {
		text := strings.Join(wordTexts(line), " ")
		if i := strings.IndexAny(text, "0123456789"); i >= 0 {
			text = text[:i]
		}
		candidate := cleanRegion(text)
		normalized := lettersOnly(candidate)
		lower := strings.ToLower(candidate)
		if len(normalized) < 5 || strings.Contains(lower, "utc") || strings.Contains(lower, "weather") {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func matchKnownRegion(candidates []string) string {
	best, bestScore := "", 0.0
	wordRE := regexp.MustCompile(`[A-Za-z]+`)
	for _, candidate := range candidates {
		candidateWords := wordRE.FindAllString(strings.ToLower(candidate), -1)
		for _, region := range knownRegions {
			regionWords := wordRE.FindAllString(strings.ToLower(region), -1)
			wordScore := 0.0
			for _, a := range candidateWords {
				if len(a) < 4 {
					continue
				}
				for _, b := range regionWords {
					if score := substringSimilarity(a, b); score > wordScore {
						wordScore = score
					}
				}
			}
			wholeScore := similarity(lettersOnly(candidate), lettersOnly(region))
			score := wholeScore*.65 + wordScore*.35
			if wordScore >= .62 && score > bestScore {
				best, bestScore = region, score
			}
		}
	}
	if bestScore < .42 {
		return ""
	}
	return best
}

func substringSimilarity(candidate, target string) float64 {
	best := similarity(candidate, target)
	for length := len(target) - 2; length <= len(target)+2; length++ {
		if length < 4 || length > len(candidate) {
			continue
		}
		for start := 0; start+length <= len(candidate); start++ {
			if score := similarity(candidate[start:start+length], target); score > best {
				best = score
			}
		}
	}
	return best
}

func lettersOnly(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	distance := levenshtein(a, b)
	longest := len(a)
	if len(b) > longest {
		longest = len(b)
	}
	return 1 - float64(distance)/float64(longest)
}

func levenshtein(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min3Int(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func min3Int(a, b, c int) int {
	if a < b {
		b = a
	}
	if c < b {
		return c
	}
	return b
}

func preprocessHeader(src image.Image) image.Image {
	b := src.Bounds()
	scale := 3
	dst := image.NewGray(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			gray := color.GrayModel.Convert(src.At(b.Min.X+x/scale, b.Min.Y+y/scale)).(color.Gray).Y
			if gray >= 178 {
				dst.SetGray(x, y, color.Gray{Y: 255})
			} else {
				dst.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return dst
}

func cropHeader(src image.Image) image.Image {
	b := src.Bounds()
	r := normalizedRect(b, .75261, .05093, .98959, .21297)
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			dst.Set(x, y, src.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return dst
}

func preprocessRegion(src image.Image, threshold uint8) image.Image {
	b := src.Bounds()
	r := normalizedRect(b, .75261, .12038, .90886, .17130)
	scale := 4
	dst := image.NewGray(image.Rect(0, 0, r.Dx()*scale, r.Dy()*scale))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			blue := lanczosBlue(src, r, (float64(x)+.5)/float64(scale)-.5, (float64(y)+.5)/float64(scale)-.5)
			if blue >= threshold {
				dst.SetGray(x, y, color.Gray{Y: 255})
			} else {
				dst.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return dst
}

func lanczosBlue(src image.Image, bounds image.Rectangle, x, y float64) uint8 {
	var value, weightSum float64
	xBase, yBase := int(math.Floor(x)), int(math.Floor(y))
	for sy := yBase - 2; sy <= yBase+3; sy++ {
		wy := lanczosKernel(y - float64(sy))
		if wy == 0 {
			continue
		}
		cy := sy
		if cy < 0 {
			cy = 0
		}
		if cy >= bounds.Dy() {
			cy = bounds.Dy() - 1
		}
		for sx := xBase - 2; sx <= xBase+3; sx++ {
			wx := lanczosKernel(x - float64(sx))
			if wx == 0 {
				continue
			}
			cx := sx
			if cx < 0 {
				cx = 0
			}
			if cx >= bounds.Dx() {
				cx = bounds.Dx() - 1
			}
			_, _, blue := rgb8(src.At(bounds.Min.X+cx, bounds.Min.Y+cy))
			weight := wx * wy
			value += float64(blue) * weight
			weightSum += weight
		}
	}
	if weightSum != 0 {
		value /= weightSum
	}
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value + .5)
}

func lanczosKernel(x float64) float64 {
	x = math.Abs(x)
	if x == 0 {
		return 1
	}
	if x >= 3 {
		return 0
	}
	return (math.Sin(math.Pi*x) / (math.Pi * x)) * (math.Sin(math.Pi*x/3) / (math.Pi * x / 3))
}

func iconRatios(img image.Image, centerY int) (float64, float64) {
	b := img.Bounds()
	centerY = findIconCenter(img, centerY)
	r := normalizedRect(b, .9425, float64(centerY-16-b.Min.Y)/float64(b.Dy()), .9792, float64(centerY+16-b.Min.Y)/float64(b.Dy()))
	var n, yellow, blue int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r, g, bl := rgb8(img.At(x, y))
			n++
			if r > 170 && g > 135 && bl < 130 && float64(r) > float64(bl)*1.45 {
				yellow++
			}
			if bl > 130 && float64(bl) > float64(r)*1.2 {
				blue++
			}
		}
	}
	return ratio(yellow, n), ratio(blue, n)
}

func findIconCenter(img image.Image, approximateY int) int {
	b := img.Bounds()
	x0 := b.Min.X + int(float64(b.Dx())*.9425)
	x1 := b.Min.X + int(float64(b.Dx())*.9792)
	bestY, bestScore := approximateY, -1
	for y := approximateY - 24; y <= approximateY+24; y++ {
		if y < b.Min.Y || y >= b.Max.Y {
			continue
		}
		score := 0
		for x := x0; x < x1; x++ {
			r, g, blue := rgb8(img.At(x, y))
			isYellow := r > 170 && g > 125 && blue < 150 && float64(r) > float64(blue)*1.35
			isBlue := blue > 125 && float64(blue) > float64(r)*1.15
			if isYellow || isBlue {
				score++
			}
		}
		if score > bestScore {
			bestY, bestScore = y, score
		}
	}
	return bestY
}

func classifyWeather(yellow, blue float64) string {
	switch {
	case yellow >= .045 && yellow > blue*1.5:
		return "sunny"
	case yellow >= .006 && blue >= .008:
		return "thunderstorm"
	case blue >= .07:
		return "rainy"
	case blue >= .012:
		return "snowy"
	default:
		return "special_unknown"
	}
}

func wordTexts(line []word) []string {
	result := make([]string, len(line))
	for i, w := range line {
		result[i] = w.text
	}
	return result
}

func lineCenter(line []word) int {
	minY, maxY := line[0].y, line[0].y+line[0].h
	for _, w := range line[1:] {
		if w.y < minY {
			minY = w.y
		}
		if w.y+w.h > maxY {
			maxY = w.y + w.h
		}
	}
	return (minY + maxY) / 2
}
