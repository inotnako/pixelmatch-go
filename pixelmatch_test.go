package pixelmatch

import (
	"image"
	"image/png"
	"os"
	"testing"
)

func TestMatch(t *testing.T) {
	fileA, err := os.Open("./testdata/img1.png")
	if err != nil {
		t.Fatal(err)
	}
	defer fileA.Close()

	fileB, err := os.Open("./testdata/img2.png")
	if err != nil {
		t.Fatal(err)
	}
	defer fileB.Close()

	imgA, _, err := image.Decode(fileA)
	if err != nil {
		t.Fatal(err)
	}

	imgB, _, err := image.Decode(fileB)
	if err != nil {
		t.Fatal(err)
	}

	output := image.NewRGBA(image.Rect(0, 0, imgA.Bounds().Max.X, imgA.Bounds().Max.Y))

	diffCount, err := Match(imgA, imgB, output)
	if err != nil {
		t.Error("Unexpected error:", err)
	}
	if diffCount != 146355 {
		t.Errorf("Expected 146355, got - %d", diffCount)
	}

	f, err := os.Create("./testdata/output.png")
	if err != nil {
		t.Fatal(err)
	}

	if err := png.Encode(f, output); err != nil {
		f.Close()
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
