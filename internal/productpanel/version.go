package productpanel

import (
	"context"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"time"
)

func BuildLabel(revision string) string {
	if revision == "development" {
		return "개발 빌드"
	}
	if len(revision) != 40 && len(revision) != 64 {
		return "확인 불가"
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return "확인 불가"
	}
	return revision[:8]
}

func CurrentBuildLabel() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "확인 불가"
	}
	build, dirty := "development", false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			build = setting.Value
		}
		if setting.Key == "vcs.modified" {
			dirty = setting.Value == "true"
		}
	}
	label := BuildLabel(build)
	if dirty {
		label += " (수정됨)"
	}
	return label
}

// Version lookup is display-only, after the requested operation. Failure cannot
// stop disconnect or silently reuse metadata from a previous agent process.
func AgentVersionLabel(ctx context.Context, open func() (RuntimeClient, error)) string {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	client, err := open()
	if err != nil {
		return "확인 불가"
	}
	info, err := client.Runtime(ctx)
	if err != nil || info.Generation < 1 || info.Generation > 1000000 {
		return "확인 불가"
	}
	return fmt.Sprintf("g%d · %s", info.Generation, BuildLabel(info.Build))
}

func VersionCaption(app, agent string) string {
	if agent == "" {
		agent = "확인 중"
	}
	return "관리 앱: " + app + "  |  에이전트: " + agent
}
