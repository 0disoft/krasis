package linuxclipboard

import "unicode/utf8"

func latin1Text(data []byte) string {
	buf := make([]byte, 0, len(data))
	for _, b := range data {
		buf = utf8.AppendRune(buf, rune(b))
	}
	return string(buf)
}

func textLatin1(text string) ([]byte, bool) {
	buf := make([]byte, 0, len(text))
	for _, r := range text {
		if r > 255 {
			return nil, false
		}
		buf = append(buf, byte(r))
	}
	return buf, true
}
