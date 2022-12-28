package pixelmatch

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
)

type Options struct {
	// matching threshold (0 to 1); smaller is more sensitive
	threshold float64

	// whether to skip anti-aliasing detection
	includeAA bool

	// opacity of original image in diff output
	alpha float32

	// color of anti-aliased pixels in diff output
	aaColor color.Color

	// color of different pixels in diff output
	diffColor color.Color

	// whether to detect dark on light differences between img1 and img2
	//  and set an alternative color to differentiate between the two
	diffColorAlt color.Color

	// draw the diff over a transparent background (a mask)
	diffMask bool
}

var defaultOptions = Options{
	threshold: 0.1,
	includeAA: false,
	alpha:     0.1,
	aaColor: &color.RGBA{
		R: 255,
		G: 255,
		B: 0,
		A: 255,
	},

	diffColor: &color.RGBA{
		R: 255,
		G: 0,
		B: 0,
		A: 255,
	},

	diffColorAlt: nil,
	diffMask:     false,
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

func Match(img1, img2 image.Image, output *image.RGBA) (uint64, error) {

	if err := checkImages([]image.Image{img1, img2, output}...); err != nil {
		return 0, err
	}

	//TODO: added here fast checking if the same images

	options := defaultOptions

	// maximum acceptable square distance between two colors;
	// 35215 is the maximum possible value for the YIQ difference metric
	maxDelta := float64(35215.0) * options.threshold * options.threshold
	var (
		diff uint64 = 0
		h           = output.Bounds().Max.Y
		w           = output.Bounds().Max.X
	)

	// compare each pixel of one image against the other one
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pos := image.Pt(x, y)

			// squared YUV distance between colors at this pixel position, negative if the img2 pixel is darker
			delta := colorDelta(img1, img2, pos, pos, false)

			// the color difference is above the threshold
			if math.Abs(delta) > maxDelta {
				// check it's a real rendering difference or just anti-aliasing
				if !options.includeAA && (antialiased(img1, img2, x, y, w, h) || antialiased(img2, img1, x, y, w, h)) {
					// one of the pixels is anti-aliasing; draw as yellow and do not count as difference
					// note that we do not include such pixels in a mask
					if !options.diffMask {
						drawPixel(output, pos, options.aaColor)
					}

				} else {
					// found substantial difference not caused by anti-aliasing; draw it as such
					// ...(delta < 0 && options.diffColorAlt ||
					drawPixel(output, pos, options.diffColor)
					diff++
				}

			} else if output != nil {
				// pixels are similar; draw background as grayscale image blended with white
				if !options.diffMask {
					drawGrayPixel(img1, pos, options.alpha, output)
				}
			}
		}
	}

	return diff, nil
}

func drawPixel(img *image.RGBA, pos image.Point, c color.Color) {
	img.Set(pos.X, pos.Y, c)
}

func drawGrayPixel(img image.Image, pos image.Point, alpha float32, out *image.RGBA) {
	r, g, b, a := img.At(pos.X, pos.Y).RGBA()
	val := blend(uint32(rgb2y(r, g, b)), uint32((alpha*float32(a))/255))
	drawPixel(out, pos, &color.RGBA{
		R: uint8(val),
		G: uint8(val),
		B: uint8(val),
		A: 255,
	})
}

// calculate color difference according to the paper "Measuring perceived color difference
// using YIQ NTSC transmission color space in mobile applications" by Y. Kotsarenko and F. Ramos
func colorDelta(img1, img2 image.Image, k, m image.Point, yOnly bool) float64 {
	var (
		r1, g1, b1, a1 = img1.At(k.X, k.Y).RGBA()
		r2, g2, b2, a2 = img2.At(m.X, m.Y).RGBA()
	)

	if a1 == a2 && r1 == r2 && g1 == g2 && b1 == b2 {
		return 0
	}

	if a1 < 255 {
		a1 /= 255
		r1 = blend(r1, a1)
		g1 = blend(g1, a1)
		b1 = blend(b1, a1)
	}

	if a2 < 255 {
		a2 /= 255
		r2 = blend(r2, a2)
		g2 = blend(g2, a2)
		b2 = blend(b2, a2)
	}

	var (
		y1 = rgb2y(r1, g1, b1)
		y2 = rgb2y(r2, g2, b2)
		y  = y1 - y2
	)

	// brightness difference only
	if yOnly {
		return y
	}

	var (
		i = rgb2i(r1, g1, b1) - rgb2i(r2, g2, b2)
		q = rgb2q(r1, g1, b1) - rgb2q(r2, g2, b2)
	)

	delta := 0.5053*y*y + 0.299*i*i + 0.1957*q*q

	// encode whether the pixel lightens or darkens in the sign
	if y1 > y2 {
		return -delta
	}

	return delta
}

func rgb2y(r, g, b uint32) float64 {
	return float64(r)*0.29889531 + float64(g)*0.58662247 + float64(b)*0.11448223
}
func rgb2i(r, g, b uint32) float64 {
	return float64(r)*0.59597799 - float64(g)*0.27417610 - float64(b)*0.32180189
}
func rgb2q(r, g, b uint32) float64 {
	return float64(r)*0.21147017 - float64(g)*0.52261711 + float64(b)*0.31114694
}

// blend semi-transparent color with white
func blend(c, a uint32) uint32 {
	return 255 + (c-255)*a
}

// check if a pixel is likely a part of anti-aliasing;
// based on "Anti-aliased Pixel and Intensity Slope Detector" paper by V. Vysniauskas, 2009
func antialiased(img, img2 image.Image, x1, y1, width, height int) bool {
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
			delta := colorDelta(img, img, image.Pt(x1, y1), image.Pt(x, y), true)

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
	return (hasManySiblings(img, minX, minY, width, height) && hasManySiblings(img2, minX, minY, width, height)) ||
		(hasManySiblings(img, maxX, maxY, width, height) && hasManySiblings(img2, maxX, maxY, width, height))
}

func colorEq(c1, c2 color.Color) bool {
	var (
		r1, g1, b1, a1 = c1.RGBA()
		r2, g2, b2, a2 = c2.RGBA()
	)
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// check if a pixel has 3+ adjacent pixels of the same color.
func hasManySiblings(img image.Image, x1, y1, width, height int) bool {
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

			if colorEq(img.At(x1, y1), img.At(x, y)) {
				zeroes++
			}

			if zeroes > 2 {
				return true
			}

		}
	}

	return false
}
