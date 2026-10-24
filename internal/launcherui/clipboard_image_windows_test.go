//go:build windows

package launcherui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"

	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestPNGToClipboardDIB(t *testing.T) {
	bitmap := image.NewRGBA(image.Rect(0, 0, 2, 1))
	bitmap.Set(0, 0, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, bitmap); err != nil {
		t.Fatal(err)
	}
	dib, err := dibFromPNG(encoded.Bytes())
	if err != nil || !peerupdate.ValidClipboardImage("dib", dib) ||
		binary.LittleEndian.Uint32(dib[4:8]) != 2 || !bytes.Equal(dib[40:44], []byte{0x56, 0x34, 0x12, 0xff}) {
		t.Fatal("PNG clipboard DIB conversion", err)
	}
}

func TestPNGToClipboardDIBPixelLayout(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4} {
		bounds := image.Rect(2, 3, 5, 3+height)
		cases := []struct {
			name   string
			bitmap draw.Image
		}{
			{"rgba", image.NewRGBA(bounds)},
			{"nrgba", image.NewNRGBA(bounds)},
			{"nrgba64", image.NewNRGBA64(bounds)},
			{"gray16", image.NewGray16(bounds)},
			{"palette", image.NewPaletted(bounds, color.Palette{
				color.NRGBA{R: 255, A: 255}, color.NRGBA{G: 255, A: 128},
				color.NRGBA{B: 255, A: 64}, color.NRGBA{R: 255, B: 255, A: 0},
			})},
		}
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/height=%d", tc.name, height), func(t *testing.T) {
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						tc.bitmap.Set(x, y, color.NRGBA{
							R: byte(x * 51), G: byte(y * 37), B: byte((x + y) * 23),
							A: []byte{0, 64, 128, 255}[(x+y)%4],
						})
					}
				}
				var encoded bytes.Buffer
				if err := png.Encode(&encoded, tc.bitmap); err != nil {
					t.Fatal(err)
				}
				decoded, err := png.Decode(bytes.NewReader(encoded.Bytes()))
				if err != nil {
					t.Fatal(err)
				}
				dib, err := dibFromPNG(encoded.Bytes())
				if err != nil || !peerupdate.ValidClipboardImage("dib", dib) || len(dib) != 40+3*height*4 ||
					binary.LittleEndian.Uint32(dib[4:8]) != 3 || binary.LittleEndian.Uint32(dib[8:12]) != uint32(height) {
					t.Fatal("invalid DIB layout", err)
				}
				for y := 0; y < height; y++ {
					for x := 0; x < 3; x++ {
						r, g, b, a := decoded.At(x, y).RGBA()
						offset := 40 + ((height-1-y)*3+x)*4
						want := []byte{byte(b >> 8), byte(g >> 8), byte(r >> 8), byte(a >> 8)}
						if got := dib[offset : offset+4]; !bytes.Equal(got, want) {
							t.Fatalf("pixel (%d,%d): got %v, want %v", x, y, got, want)
						}
					}
				}
			})
		}
	}
}
