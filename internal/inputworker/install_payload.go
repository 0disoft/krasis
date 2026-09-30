package inputworker

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/pe"
	"encoding/hex"
)

// Constructed by a trusted installer from its embedded, independently approved
// images. There is deliberately no path, manifest or RPC-based constructor.
type installPayload struct {
	images installImages
	agent  []byte
	input  []byte
}

func pinnedInstallPayload(agent, input []byte, expected installImages) (*installPayload, error) {
	if !expected.valid() || sha256.Sum256(agent) != expected.Agent || sha256.Sum256(input) != expected.Service {
		return nil, ErrAuthority
	}
	var revision string
	for index, image := range [][]byte{agent, input} {
		if len(image) == 0 || len(image) > 60<<20 {
			return nil, ErrAuthority
		}
		executable, err := pe.NewFile(bytes.NewReader(image))
		if err != nil {
			return nil, ErrAuthority
		}
		_, amd64 := executable.OptionalHeader.(*pe.OptionalHeader64)
		machine := executable.FileHeader.Machine
		_ = executable.Close()
		if !amd64 || machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			return nil, ErrAuthority
		}
		info, err := buildinfo.Read(bytes.NewReader(image))
		if err != nil {
			return nil, ErrAuthority
		}
		command := "krasis-agent"
		if index == 1 {
			command = "krasis-input"
		}
		if info.Path != "github.com/0disoft/krasis/cmd/"+command {
			return nil, ErrAuthority
		}
		settings := map[string]string{}
		for _, setting := range info.Settings {
			settings[setting.Key] = setting.Value
		}
		revisionBytes, revisionErr := hex.DecodeString(settings["vcs.revision"])
		if settings["GOOS"] != "windows" || settings["GOARCH"] != "amd64" || settings["CGO_ENABLED"] != "0" || settings["vcs"] != "git" || settings["vcs.modified"] != "false" || revisionErr != nil || len(revisionBytes) != 20 {
			return nil, ErrAuthority
		}
		if index == 0 {
			revision = settings["vcs.revision"]
		} else if revision != settings["vcs.revision"] {
			return nil, ErrAuthority
		}
	}
	return &installPayload{images: expected, agent: bytes.Clone(agent), input: bytes.Clone(input)}, nil
}

func (payload *installPayload) valid() bool {
	return payload != nil && payload.images.valid() && len(payload.agent) > 0 && len(payload.input) > 0 &&
		sha256.Sum256(payload.agent) == payload.images.Agent && sha256.Sum256(payload.input) == payload.images.Service
}
