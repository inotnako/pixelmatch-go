package pixelmatch

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"sync/atomic"
)

type Options struct {
	// matching threshold (0 to 1); smaller is more sensitive
	threshold float64

	// whether to skip anti-aliasing detection
	includeAA bool

	// opacity of original image in diff output
	alpha float32

	// color of anti-aliased pixels in diff output
	aaColor color.NRGBA

	// color of different pixels in diff output
	diffColor color.NRGBA

	// whether to detect dark on light differences between img1 and img2
	//  and set an alternative color to differentiate between the two
	diffColorAlt color.Color

	// draw the diff over a transparent background (a mask)
	diffMask bool
}

var defaultOptions = Options{
	threshold: 0.1,
	includeAA: true,
	alpha:     0.1,

	aaColor: color.NRGBA{
		R: 255,
		G: 255,
		B: 0,
		A: 255,
	},

	diffColor: color.NRGBA{
		R: 255,
		G: 0,
		B: 0,
		A: 255,
	},

	diffColorAlt: nil,
	diffMask:     true,
}

func isEmptyImg(img image.Image) bool {
	return img == nil || img.Bounds().Empty()
}

var (
	ErrEmptyImage = errors.New("image is empty")
	ErrImageSize  = errors.New("size of images must be equals")
)

func indexImgStr(i int) string {
	switch i {
	case 0:
		return "first img"
	case 1:
		return "second img"
	case 2:
		return "output img"
	default:
		return fmt.Sprint(i) + " img"
	}
}

func checkImages(imgs ...image.Image) error {
	var (
		emptyImgs          []string
		notEqualImgsBySize []string
	)

	for i := range imgs {
		if isEmptyImg(imgs[i]) {
			emptyImgs = append(emptyImgs, indexImgStr(i))
		}
	}

	if len(emptyImgs) > 0 {
		return fmt.Errorf("%w: images: %s", ErrEmptyImage, strings.Join(emptyImgs, `,`))
	}

	for i := 0; i < len(imgs)-1; i++ {
		if !imgs[i].Bounds().Eq(imgs[i+1].Bounds()) {
			notEqualImgsBySize = append(
				notEqualImgsBySize,
				fmt.Sprintf(
					"%q (%s) != %q (%s)",
					indexImgStr(i),
					imgs[i].Bounds().String(),
					indexImgStr(i+1),
					imgs[i+1].Bounds().String(),
				),
			)
		}
	}

	if len(notEqualImgsBySize) > 0 {
		return fmt.Errorf(
			"%w: images: %s",
			ErrImageSize,
			strings.Join(notEqualImgsBySize, `,`),
		)
	}

	return nil
}

func Diff(img1, img2 image.Image, output *image.NRGBA) (uint64, error) {

	if err := checkImages([]image.Image{img1, img2, output}...); err != nil {
		return 0, err
	}

	options := defaultOptions

	img1Obj, _ := img1.(*image.NRGBA)
	img2Obj, _ := img2.(*image.NRGBA)

	// maximum acceptable square distance between two colors;
	// 35215 is the maximum possible value for the YIQ difference metric
	maxDelta := float64(35215.0) * options.threshold * options.threshold
	var (
		diff uint64 = 0
		h           = output.Bounds().Max.Y
		w           = output.Bounds().Max.X
		wg          = sync.WaitGroup{}
	)

	processSubImage := func(a, b *image.NRGBA, rectangle image.Rectangle) {
		defer wg.Done()

		var (
			cc1, cc2 [4]uint8
		)
		containerDiff := uint64(0)
		// compare each pixel of one image against the other one
		for y := rectangle.Min.Y; y < rectangle.Max.Y; y++ {
			for x := rectangle.Min.X; x < rectangle.Max.X; x++ {
				cc1 = getColor(a, x, y)
				cc2 = getColor(b, x, y)

				// squared YUV distance between colors at this pixel position, negative if the img2 pixel is darker
				delta := colorDelta(cc1, cc2, false)

				// the color difference is above the threshold
				if math.Abs(delta) > maxDelta {
					// check it's a real rendering difference or just anti-aliasing
					if !options.includeAA && (antialiased(a, b, x, y, w, h) || antialiased(a, b, x, y, w, h)) {
						// one of the pixels is anti-aliasing; draw as yellow and do not count as difference
						// note that we do not include such pixels in a mask
						if !options.diffMask {
							output.SetNRGBA(x, y, options.aaColor)
						}

					} else {
						// found substantial difference not caused by anti-aliasing; draw it as such
						output.SetNRGBA(x, y, options.diffColor)
						containerDiff++
					}

				} else if !options.diffMask {
					// pixels are similar; draw background as grayscale image blended with white
					output.SetNRGBA(x, y, grayColor(cc1, options.alpha))
				}
			}
		}
		atomic.AddUint64(&diff, containerDiff)
	}

	containerW := 2000
	containerH := 2000
	if containerH > h {
		containerH = h - 1
	}
	if containerW > w {
		containerW = w - 1
	}

	/*
				((0,0),(100,100)) | ((100,0),(200,100)) | ((200,0),(300,100)) ....
				((0,0),(100,100)) | ((100,0),(200,100)) | ((200,0),(300,100)) ....
		        ....
	*/

	for x0 := 0; x0 < w-containerW; x0 += containerW {
		for y0 := 0; y0 < h-containerH; y0 += containerH {
			wg.Add(1)
			x1 := x0 + containerW
			y1 := y0 + containerW

			if x1+containerW > w {
				x1 = w
			}
			if y1+containerH > h {
				y1 = h
			}
			rect := image.Rect(x0, y0, x1, y1)

			go processSubImage(
				img1Obj.SubImage(rect).(*image.NRGBA),
				img2Obj.SubImage(rect).(*image.NRGBA),
				rect,
			)
		}
	}

	wg.Wait()

	return diff, nil
}

func grayColor(c [4]uint8, alpha float32) color.NRGBA {
	val := blend(
		uint8(rgb2y(c[0], c[1], c[2])),
		uint8((alpha*float32(c[3]))/255),
	)
	return color.NRGBA{
		R: val,
		G: val,
		B: val,
		A: 255,
	}
}

// calculate color difference according to the paper "Measuring perceived color difference
// using YIQ NTSC transmission color space in mobile applications" by Y. Kotsarenko and F. Ramos
func colorDelta(c1, c2 [4]uint8, yOnly bool) float64 {
	if colorEq(c1, c2) {
		return 0
	}

	if c1[3] < 255 {
		c1[3] /= 255
		c1[0] = blend(c1[0], c1[3])
		c1[1] = blend(c1[1], c1[3])
		c1[2] = blend(c1[2], c1[3])
	}

	if c2[3] < 255 {
		c2[3] /= 255
		c2[0] = blend(c2[0], c2[3])
		c2[1] = blend(c2[1], c2[3])
		c2[2] = blend(c2[2], c2[3])
	}

	var (
		y1 = rgb2y(c1[0], c1[1], c1[2])
		y2 = rgb2y(c2[0], c2[1], c2[2])
		y  = y1 - y2
	)

	// brightness difference only
	if yOnly {
		return y
	}

	var (
		i = rgb2i(c1[0], c1[1], c1[2]) - rgb2i(c2[0], c2[1], c2[2])
		q = rgb2q(c1[0], c1[1], c1[2]) - rgb2q(c2[0], c2[1], c2[2])
	)

	delta := 0.5053*y*y + 0.299*i*i + 0.1957*q*q

	// encode whether the pixel lightens or darkens in the sign
	if y1 > y2 {
		return -delta
	}

	return delta
}

func rgb2y(r, g, b uint8) float64 {
	return float64(r)*0.29889531 + float64(g)*0.58662247 + float64(b)*0.11448223
}
func rgb2i(r, g, b uint8) float64 {
	return float64(r)*0.59597799 - float64(g)*0.27417610 - float64(b)*0.32180189
}
func rgb2q(r, g, b uint8) float64 {
	return float64(r)*0.21147017 - float64(g)*0.52261711 + float64(b)*0.31114694
}

// blend semi-transparent color with white
func blend(c, a uint8) uint8 {
	return 255 + (c-255)*a
}

// check if a pixel is likely a part of anti-aliasing;
// based on "Anti-aliased Pixel and Intensity Slope Detector" paper by V. Vysniauskas, 2009
func antialiased(a, b *image.NRGBA, x1, y1, width, height int) bool {
	var (
		x0                             = int(math.Max(float64(x1-1), 0))
		y0                             = int(math.Max(float64(y1-1), float64(0)))
		x2                             = int(math.Min(float64(x1+1), float64(width-1)))
		y2                             = int(math.Min(float64(y1+1), float64(height-1)))
		zeroes                         = 0
		min                    float64 = 0
		max                    float64 = 0
		minX, minY, maxX, maxY int
	)

	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}

	// go through 8 adjacent pixels
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}

			// brightness delta between the center pixel and adjacent one
			delta := colorDelta(getColor(a, x1, y1), getColor(b, x, y), true)

			// count the number of equal, darker and brighter adjacent pixels
			if delta == 0 {
				zeroes++
				// if found more than 2 equal siblings, it's definitely not anti-aliasing
				if zeroes > 2 {
					return false
				}

				// remember the darkest pixel
			} else if delta < min {
				min = delta
				minX = x
				minY = y

				// remember the brightest pixel
			} else if delta > max {
				max = delta
				maxX = x
				maxY = y
			}
		}
	}

	// if there are no both darker and brighter pixels among siblings, it's not anti-aliasing
	if min == 0 || max == 0 {
		return false
	}

	// if either the darkest or the brightest pixel has 3+ equal siblings in both images
	// (definitely not anti-aliased), this pixel is anti-aliased
	return (hasManySiblings(a, minX, minY, width, height) && hasManySiblings(b, minX, minY, width, height)) ||
		(hasManySiblings(a, maxX, maxY, width, height) && hasManySiblings(b, maxX, maxY, width, height))
}

func getColor(img *image.NRGBA, x, y int) (c [4]uint8) {
	i := img.PixOffset(x, y)
	copy(c[:], img.Pix[i:i+4:i+4])

	return
}

func colorEq(c1, c2 [4]uint8) bool {
	return c1 == c2
	//return c1[0] == c2[0] && c1[1] == c2[1] && c1[2] == c2[2] && c1[3] == c2[3]
}

// check if a pixel has 3+ adjacent pixels of the same color.
func hasManySiblings(a *image.NRGBA, x1, y1, width, height int) bool {
	var (
		x0     = int(math.Max(float64(x1-1), 0))
		y0     = int(math.Max(float64(y1-1), float64(0)))
		x2     = int(math.Min(float64(x1+1), float64(width-1)))
		y2     = int(math.Min(float64(y1+1), float64(height-1)))
		zeroes = 0
	)

	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}

	// go through 8 adjacent pixels
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}

			if colorEq(getColor(a, x1, y1), getColor(a, x, y)) {
				zeroes++
			}

			if zeroes > 2 {
				return true
			}
		}
	}

	return false
}
