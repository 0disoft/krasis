//go:build linux

package residentagent

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"sync"
	"syscall"
)

func linuxX11SystemCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.CommandContext(ctx, name, args...)
	}
	return exec.CommandContext(ctx, "sudo", append([]string{"-n", name}, args...)...)
}

// The disposable accounts/groups are provisioned before the suite. Resolve
// their credentials once, rather than repeating NSS/PAM work for every CLI.
var linuxX11Credentials sync.Map

func linuxX11UserCredential(name string) (*syscall.Credential, error) {
	if cached, ok := linuxX11Credentials.Load(name); ok {
		return cached.(*syscall.Credential), nil
	}
	account, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(account.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	groupIDs, err := account.GroupIds()
	if err != nil {
		return nil, err
	}
	credential := &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	for _, value := range groupIDs {
		group, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, err
		}
		credential.Groups = append(credential.Groups, uint32(group))
	}
	value, _ := linuxX11Credentials.LoadOrStore(name, credential)
	return value.(*syscall.Credential), nil
}
