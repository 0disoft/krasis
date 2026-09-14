// Package pindiscovery locates explicit, short-lived invitations, not trusted peers.
package pindiscovery

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
	"time"
)

const port = 45873
const prefix = "krasis-pin-discover-v1:"

type Candidate struct {
	Address string
	ID      string
}
type announcement struct {
	Nonce string `json:"nonce"`
	ID    string `json:"id"`
	Port  int    `json:"port"`
}

func validID(id string) bool {
	v, err := hex.DecodeString(id)
	return err == nil && len(v) == 16 && strings.ToLower(id) == id
}

// Advertise responds only while the caller's explicit invitation is active.
func Advertise(ctx context.Context, id string, tcpPort int) (io.Closer, error) {
	return advertiseAt(ctx, id, tcpPort, &net.UDPAddr{Port: port})
}

func advertiseAt(ctx context.Context, id string, tcpPort int, address *net.UDPAddr) (io.Closer, error) {
	if !validID(id) || tcpPort < 1 || tcpPort > 65535 {
		return nil, errors.New("invalid invitation")
	}
	conn, err := net.ListenUDP("udp4", address)
	if err != nil {
		return nil, err
	}
	go func() {
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		defer stop()
		var buf [256]byte
		var last time.Time
		for {
			n, peer, err := conn.ReadFromUDP(buf[:])
			if err != nil {
				return
			}
			if (!peer.IP.IsPrivate() && !peer.IP.IsLoopback()) || time.Since(last) < 50*time.Millisecond {
				continue
			}
			request := string(buf[:n])
			if !strings.HasPrefix(request, prefix) || !validID(strings.TrimPrefix(request, prefix)) {
				continue
			}
			last = time.Now()
			data, _ := json.Marshal(announcement{strings.TrimPrefix(request, prefix), id, tcpPort})
			conn.WriteToUDP(data, peer)
		}
	}()
	return conn, nil
}

// Discover treats replies as untrusted routing hints. PIN authentication remains mandatory.
func Discover(ctx context.Context) ([]Candidate, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err = enableBroadcast(conn); err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(random[:])
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(1500 * time.Millisecond)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.To4() == nil || !network.IP.IsPrivate() {
			continue
		}
		ip := network.IP.To4()
		mask := network.Mask
		if len(mask) != 4 {
			continue
		}
		broadcast := net.IPv4(ip[0]|^mask[0], ip[1]|^mask[1], ip[2]|^mask[2], ip[3]|^mask[3])
		conn.WriteToUDP([]byte(prefix+nonce), &net.UDPAddr{IP: broadcast, Port: port})
	}
	var result []Candidate
	seen := map[string]bool{}
	var buf [512]byte
	for len(result) < 8 {
		n, peer, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if e, ok := err.(net.Error); ok && e.Timeout() {
				break
			}
			return nil, err
		}
		candidate, ok := decode(buf[:n], nonce, peer.IP)
		if !ok || seen[candidate.Address] {
			continue
		}
		seen[candidate.Address] = true
		result = append(result, candidate)
	}
	return result, nil
}

func decode(data []byte, nonce string, ip net.IP) (Candidate, bool) {
	var value announcement
	if (!ip.IsPrivate() && !ip.IsLoopback()) || len(data) > 256 || json.Unmarshal(data, &value) != nil || value.Nonce != nonce || !validID(value.ID) || value.Port < 1 || value.Port > 65535 {
		return Candidate{}, false
	}
	return Candidate{net.JoinHostPort(ip.String(), strconv.Itoa(value.Port)), value.ID}, true
}
