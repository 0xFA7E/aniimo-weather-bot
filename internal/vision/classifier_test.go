package vision

import (
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifierFixtures(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"unrelated.jpg", false},
		{"weather_controller.jpg", true},
		{"weather_keyboard.jpg", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := openOptionalFixture(t, tt.name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := jpeg.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			got := Classify(img)
			if got.Candidate != tt.want {
				t.Fatalf("Candidate=%t, want %t (%s)", got.Candidate, tt.want, got.Reason())
			}
		})
	}
}

func openOptionalFixture(t *testing.T, name string) (*os.File, error) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", name)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		t.Skipf("optional private OCR fixture %s is not present", name)
	}
	return f, err
}
