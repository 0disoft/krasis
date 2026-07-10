package buildinfo

import "runtime/debug"

type Info struct {
	Revision string `json:"revision"`
	Modified bool   `json:"modified"`
}

func Read() Info {
	result := Info{Revision: "unknown"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return result
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if setting.Value != "" {
				result.Revision = setting.Value
			}
		case "vcs.modified":
			result.Modified = setting.Value == "true"
		}
	}
	return result
}
