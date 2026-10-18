package vision

import (
	"context"
	"image/jpeg"
	"os/exec"
	"testing"
)

func TestExtractFixtures(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract is not installed")
	}
	for _, name := range []string{"weather_controller.jpg", "weather_keyboard.jpg"} {
		t.Run(name, func(t *testing.T) {
			f, err := openOptionalFixture(t, name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := jpeg.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Extract(context.Background(), img, "")
			if err != nil {
				t.Fatal(err)
			}
			if !got.Valid {
				t.Fatalf("expected valid extraction: %+v", got)
			}
			if len(got.Rows) != 6 {
				t.Fatalf("got %d rows: %+v", len(got.Rows), got.Rows)
			}
			for i, row := range got.Rows {
				want := "sunny"
				if i == 3 {
					want = "rainy"
				}
				if row.Weather != want {
					t.Errorf("row %d weather=%q want %q (yellow %.3f blue %.3f)", i, row.Weather, want, row.YellowRatio, row.BlueRatio)
				}
			}
			if name == "weather_controller.jpg" && got.Timezone != "UTC-4" {
				t.Errorf("timezone=%q", got.Timezone)
			}
		})
	}
}

func TestClassifyWeatherTypes(t *testing.T) {
	for name, tc := range map[string]struct {
		yellow, blue float64
		want         string
	}{
		"sunny":        {0.11, 0, "sunny"},
		"rainy":        {0, 0.13, "rainy"},
		"snowy":        {0, 0.03, "snowy"},
		"thunderstorm": {0.02, 0.03, "thunderstorm"},
		"unknown":      {0, 0.004, "special_unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := classifyWeather(tc.yellow, tc.blue); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestExtractRejectsUnrelated(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract is not installed")
	}
	f, err := openOptionalFixture(t, "unrelated.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Extract(context.Background(), img, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Valid {
		t.Fatalf("unrelated screenshot validated: %+v", got)
	}
}

func TestKnownRegionMatchRepairsNoisyHeader(t *testing.T) {
	for input, want := range map[string]string{
		"Russettilignianids": "Russet Highlands",
		"Se gi Ployers":      "Sea of Flowers",
		"Boryline J":         "Berylline Vale",
		"a ee ae aS":         "",
	} {
		if got := matchKnownRegion([]string{input}); got != want {
			t.Errorf("matchKnownRegion(%q)=%q want %q", input, got, want)
		}
	}
}

func TestClockRegexAcceptsOCRPeriodBeforeMeridiem(t *testing.T) {
	got := clockRE.FindString("Beast Fang Ridge 06:32:51.PM")
	if got != "06:32:51.PM" {
		t.Fatalf("got %q", got)
	}
}
