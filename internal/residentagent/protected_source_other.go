//go:build !windows

package residentagent

import (
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/wininput"
)

func protectedSessionHook(hook *wininput.SourceHook, _ func(string)) agent.SourceHook { return hook }
