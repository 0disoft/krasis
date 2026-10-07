//go:build windows && desktop

package workspacehost

import (
	"strings"
	"testing"
)

func TestTrayLocaleIsSingleLanguageAndRejectsUntrustedLabels(t *testing.T) {
	for _, locale := range []string{"ko", "en"} {
		labels, ok := trayMenuLabels(locale)
		if !ok {
			t.Fatal(locale)
		}
		for _, label := range labels {
			if label == "" || strings.Contains(label, "/") {
				t.Fatal("missing or bilingual menu item", label)
			}
		}
	}
	s := &WorkspaceService{updateTrayLabels: func([4]string) { t.Fatal("invalid locale reached native menu") }}
	for _, locale := range []string{"", "ko-KR", "ko / en", "private menu label"} {
		if s.SetTrayLocale(locale) {
			t.Fatal("unsupported locale accepted", locale)
		}
	}
}
