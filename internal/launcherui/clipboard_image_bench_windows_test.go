//go:build windows

package launcherui

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func BenchmarkDIBFromPNG1080p(b *testing.B) {
	bitmap := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			i := bitmap.PixOffset(x, y)
			bitmap.Pix[i], bitmap.Pix[i+1], bitmap.Pix[i+2], bitmap.Pix[i+3] = byte(x), byte(y), byte(x^y), 255
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, bitmap); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		dib, err := dibFromPNG(encoded.Bytes())
		if err != nil || len(dib) != 40+1920*1080*4 {
			b.Fatal("DIB conversion failed", err)
		}
	}
}
