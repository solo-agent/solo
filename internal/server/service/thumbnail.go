package service

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// defaultMaxThumbDim is the default maximum thumbnail dimension.
const defaultMaxThumbDim = 400

// GenerateThumbnail creates a thumbnail for an image file.
// If maxDim is <= 0, the default (400) is used.
// Supports JPEG and PNG inputs. The output format matches the input.
// Output path follows the pattern: {dir}/thumb_{filename}
func GenerateThumbnail(inputPath string, maxDim int) (string, error) {
	if maxDim <= 0 {
		maxDim = defaultMaxThumbDim
	}

	// Open and decode the source image.
	f, err := os.Open(inputPath)
	if err != nil {
		return "", fmt.Errorf("thumbnail: open input: %w", err)
	}
	defer f.Close()

	src, format, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("thumbnail: decode: %w", err)
	}

	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// If the image is already within maxDim, return early with the original.
	if srcW <= maxDim && srcH <= maxDim {
		return inputPath, nil
	}

	// Calculate proportional dimensions.
	newW, newH := scaleToFit(srcW, srcH, maxDim)

	// Resize using bilinear interpolation (standard library only).
	dst := resizeBilinear(src, newW, newH)

	// Determine output path: same directory, "thumb_" prefix.
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	outputPath := filepath.Join(dir, "thumb_"+base)

	out, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("thumbnail: create output: %w", err)
	}
	defer out.Close()

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(out, dst, &jpeg.Options{Quality: 85})
	case "png":
		err = png.Encode(out, dst)
	case "gif":
		// GIF resize loses animation; encode as PNG for the thumbnail.
		err = png.Encode(out, dst)
	default:
		// Fallback to PNG for unsupported formats.
		err = png.Encode(out, dst)
	}
	if err != nil {
		return "", fmt.Errorf("thumbnail: encode: %w", err)
	}

	return outputPath, nil
}

// scaleToFit computes new dimensions that fit within maxDim while preserving
// aspect ratio.
func scaleToFit(w, h, maxDim int) (int, int) {
	if w <= 0 || h <= 0 {
		return 1, 1
	}
	if w >= h {
		newW := maxDim
		newH := h * maxDim / w
		if newH < 1 {
			newH = 1
		}
		return newW, newH
	}
	newH := maxDim
	newW := w * maxDim / h
	if newW < 1 {
		newW = 1
	}
	return newW, newH
}

// resizeBilinear scales a source image to the target dimensions using
// bilinear interpolation. Uses only the Go standard library.
func resizeBilinear(src image.Image, newW, newH int) *image.RGBA {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	xRatio := float64(srcW-1) / float64(newW)
	yRatio := float64(srcH-1) / float64(newH)

	for dy := 0; dy < newH; dy++ {
		for dx := 0; dx < newW; dx++ {
			// Map destination pixel to source coordinates.
			sx := float64(dx) * xRatio
			sy := float64(dy) * yRatio

			// Integer and fractional parts for interpolation.
			x0 := int(sx)
			y0 := int(sy)
			x1 := x0 + 1
			y1 := y0 + 1
			if x1 >= srcW {
				x1 = srcW - 1
			}
			if y1 >= srcH {
				y1 = srcH - 1
			}

			xFrac := sx - float64(x0)
			yFrac := sy - float64(y0)

			// Sample 4 neighboring pixels.
			c00 := rgbaColorAt(src, x0, y0)
			c10 := rgbaColorAt(src, x1, y0)
			c01 := rgbaColorAt(src, x0, y1)
			c11 := rgbaColorAt(src, x1, y1)

			// Bilinear interpolation for each channel.
			r := uint8(bilerp(
				float64(c00.R), float64(c10.R),
				float64(c01.R), float64(c11.R),
				xFrac, yFrac,
			))
			g := uint8(bilerp(
				float64(c00.G), float64(c10.G),
				float64(c01.G), float64(c11.G),
				xFrac, yFrac,
			))
			b := uint8(bilerp(
				float64(c00.B), float64(c10.B),
				float64(c01.B), float64(c11.B),
				xFrac, yFrac,
			))
			a := uint8(bilerp(
				float64(c00.A), float64(c10.A),
				float64(c01.A), float64(c11.A),
				xFrac, yFrac,
			))

			dst.SetRGBA(dx, dy, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	return dst
}

// rgbaColorAt returns the RGBA color at the given coordinates.
func rgbaColorAt(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

// bilerp performs bilinear interpolation between four corner values.
func bilerp(c00, c10, c01, c11, xFrac, yFrac float64) float64 {
	top := c00*(1-xFrac) + c10*xFrac
	bottom := c01*(1-xFrac) + c11*xFrac
	return top*(1-yFrac) + bottom*yFrac
}
