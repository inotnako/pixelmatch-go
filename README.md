# Pixelmatch-go


```go
package main

import (
	"image"
	"image/png"
	"log"
	"os"

	"github.com/inotnako/pixelmatch-go"
)

func main() {
	fileA, err := os.Open("./testdata/img1.png")
	if err != nil {
		log.Fatal(err)
	}
	defer fileA.Close()

	fileB, err := os.Open("./testdata/img2.png")
	if err != nil {
		log.Fatal(err)
	}
	defer fileB.Close()

	imgA, _, err := image.Decode(fileA)
	if err != nil {
		log.Fatal(err)
	}

	imgB, _, err := image.Decode(fileB)
	if err != nil {
		log.Fatal(err)
	}

	output := image.NewRGBA(image.Rect(0, 0, imgA.Bounds().Max.X, imgA.Bounds().Max.Y))

	diffCount, err := pixelmatch.Match(imgA, imgB, output)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Found diff ",diffCount, "pixels")
	
	f, err := os.Create("./testdata/output.png")
	if err != nil {
		log.Fatal(err)
	}
	
	defer f.Close()

	if err := png.Encode(f, output); err != nil {
		log.Fatal(err)
	}
}
```

rewrite from https://github.com/mapbox/pixelmatch to Go