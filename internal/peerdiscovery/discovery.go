// Package peerdiscovery finds LAN routing hints. It grants no identity or permissions.
package peerdiscovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Port = 45874
const prefix = "krasis-peer-discover-v1:"

type announcement struct {
	Nonce string `json:"nonce"`
	Port  int    `json:"port"`
}

func validNonce(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && strings.ToLower(value) == value
}

type advertisement struct {
	conn *net.UDPConn
	done chan struct{}
	once sync.Once
}

func (a *advertisement) Close() error {
	var err error
	a.once.Do(func() { err = a.conn.Close() })
	<-a.done
	return err
}

// Advertise binds only the caller's existing IPv4 listener address. It discloses
// a port, not a fingerprint, alias, certificate, machine name or pairing secret.
func Advertise(ctx context.Context, inputAddress string) (io.Closer, error) {
	host, port, err := net.SplitHostPort(inputAddress)
	if err != nil {
		return nil, err
	}
	inputPort, err := strconv.Atoi(port)
	ip := net.ParseIP(host)
	if err != nil || inputPort < 1 || inputPort >= 65535 || ip.To4() == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
		return nil, errors.New("invalid discovery listener")
	}
	return advertiseAt(ctx, inputPort, &net.UDPAddr{IP: ip, Port: Port})
}

func advertiseAt(ctx context.Context, inputPort int, address *net.UDPAddr) (*advertisement, error) {
	conn, err := net.ListenUDP("udp4", address)
	if err != nil {
		return nil, err
	}
	a := &advertisement{conn: conn, done: make(chan struct{})}
	go func() {
		defer close(a.done)
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		var buffer [256]byte
		var last time.Time
		for {
			n, sender, err := conn.ReadFromUDP(buffer[:])
			if err != nil {
				return
			}
			request := string(buffer[:n])
			if (!sender.IP.IsPrivate() && !sender.IP.IsLoopback()) || time.Since(last) < 50*time.Millisecond || !strings.HasPrefix(request, prefix) || !validNonce(strings.TrimPrefix(request, prefix)) {
				continue
			}
			last = time.Now()
			data, _ := json.Marshal(announcement{Nonce: strings.TrimPrefix(request, prefix), Port: inputPort})
			_, _ = conn.WriteToUDP(data, sender)
		}
	}()
	return a, nil
}

func broadcastTargets() ([]*net.UDPAddr, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var targets []*net.UDPAddr
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.To4() == nil || !network.IP.IsPrivate() || len(network.Mask) != 4 {
			continue
		}
		ip, mask := network.IP.To4(), network.Mask
		targets = append(targets, &net.UDPAddr{IP: net.IPv4(ip[0]|^mask[0], ip[1]|^mask[1], ip[2]|^mask[2], ip[3]|^mask[3]), Port: Port})
		if len(targets) == 16 {
			break
		}
	}
	return targets, nil
}

// Discover is bounded to 1.5s and eight candidates. Every returned address must
// still pass the caller's pinned mTLS/role/current-trust verification.
func Discover(ctx context.Context) ([]string, error) {
	targets, err := broadcastTargets()
	if err != nil {
		return nil, err
	}
	return discoverAt(ctx, targets)
}

func discoverAt(ctx context.Context, targets []*net.UDPAddr) ([]string, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err = enableBroadcast(conn); err != nil {
		return nil, err
	}
	var nonceBytes [16]byte
	if _, err = rand.Read(nonceBytes[:]); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(1500 * time.Millisecond)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	for _, target := range targets {
		_, _ = conn.WriteToUDP([]byte(prefix+nonce), target)
	}
	seen := map[string]bool{}
	var candidates []string
	var buffer [512]byte
	for len(candidates) < 8 {
		n, sender, err := conn.ReadFromUDP(buffer[:])
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if e, ok := err.(net.Error); ok && e.Timeout() {
				return candidates, nil
			}
			return nil, err
		}
		candidate, ok := decode(buffer[:n], nonce, sender.IP)
		if !ok || seen[candidate] {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func decode(data []byte, nonce string, ip net.IP) (string, bool) {
	var value announcement
	if len(data) > 256 || ip.To4() == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || json.Unmarshal(data, &value) != nil || value.Nonce != nonce || !validNonce(value.Nonce) || value.Port < 1 || value.Port >= 65535 {
		return "", false
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(value.Port)), true
}
