//go:build windows

package launcherui

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
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
