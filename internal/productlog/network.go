package productlog

import "github.com/0disoft/krasis/internal/netobserve"

func init() {
	for _, code := range netobserve.Codes() {
		allowed[code] = true
	}
}
