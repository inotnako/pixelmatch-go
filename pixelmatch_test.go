package pixelmatch

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"
)

func TestDiff(t *testing.T) {
	//water-4k
	//"./testdata/water-4k.png"
	fileABytes, err := os.ReadFile("./testdata/water-4k.png")
	if err != nil {
		t.Fatal(err)
	}
	lenA := len(fileABytes)

	fileBBytes, err := os.ReadFile("./testdata/water-4k-2.png")
	if err != nil {
		t.Fatal(err)
	}

	imgA, _, err := image.Decode(bytes.NewBuffer(fileABytes))
	if err != nil {
		t.Fatal(err)
	}

	imgB, _, err := image.Decode(bytes.NewBuffer(fileBBytes))
	if err != nil {
		t.Fatal(err)
	}

	fileABytes = fileABytes[:0]
	fileBBytes = fileBBytes[:0]
	output := image.NewNRGBA(imgA.Bounds())

	diffCount, err := Diff(imgA, imgB, output)
	if err != nil {
		t.Error("Unexpected error:", err)
	}
	t.Log("diffCount", diffCount)
	if diffCount > 146355 {
		t.Errorf("Expected 146355, got - %d", diffCount)
	}

	buff := bytes.NewBuffer(make([]byte, 0, lenA))

	enc := png.Encoder{
		CompressionLevel: png.BestSpeed,
	}
	if err := enc.Encode(buff, output); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile("./testdata/output.png", buff.Bytes(), os.ModePerm); err != nil {
		t.Fatal(err)
	}
}
