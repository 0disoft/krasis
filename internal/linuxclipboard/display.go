package linuxclipboard

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
)

type displayAddress struct {
	network, address, host, number string
	screen                         int
}

func parseDisplay(value string) (displayAddress, error) {
	d := displayAddress{}
	value = strings.TrimPrefix(value, "unix/")
	colon := strings.LastIndexByte(value, ':')
	if colon != 0 {
		return d, errors.New("a local :N or unix/:N X11 DISPLAY is required")
	}
	d.host = value[:colon]
	parts := strings.Split(value[colon+1:], ".")
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 0 || n > 65535 || len(parts) > 2 {
		return d, errors.New("invalid X11 display number")
	}
	d.number = strconv.Itoa(n)
	if len(parts) == 2 {
		d.screen, err = strconv.Atoi(parts[1])
		if err != nil || d.screen < 0 || d.screen > 255 {
			return d, errors.New("invalid X11 screen")
		}
	}
	d.network, d.address = "unix", "/tmp/.X11-unix/X"+d.number
	return d, nil
}

// Xauthority records are big endian and length prefixed. Limit the entire input
// in the caller; field bounds here avoid unbounded allocation on malformed files.
func authorityCookie(r io.Reader, hostname, number string) (string, error) {
	field := func() ([]byte, error) {
		var n uint16
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return nil, err
		}
		if n > 4096 {
			return nil, errors.New("Xauthority field too large")
		}
		data := make([]byte, n)
		_, err := io.ReadFull(r, data)
		return data, err
	}
	for {
		var family uint16
		if err := binary.Read(r, binary.BigEndian, &family); err != nil {
			return "", err
		}
		address, err := field()
		if err != nil {
			return "", err
		}
		display, err := field()
		if err != nil {
			return "", err
		}
		name, err := field()
		if err != nil {
			return "", err
		}
		data, err := field()
		if err != nil {
			return "", err
		}
		if (family == 65535 || family == 256 && string(address) == hostname) &&
			(string(display) == number || len(display) == 0) && string(name) == "MIT-MAGIC-COOKIE-1" && len(data) == 16 {
			return hex.EncodeToString(data), nil
		}
	}
}
