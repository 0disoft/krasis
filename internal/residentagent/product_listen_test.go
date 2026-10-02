package residentagent

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestInterfaceRecoveryNeverWidensOrChoosesAmbiguousAddress(t *testing.T) {
	networks := func(ips ...string) []net.Addr {
		var out []net.Addr
		for _, ip := range ips {
			_, n, _ := net.ParseCIDR(ip + "/24")
			n.IP = net.ParseIP(ip)
			out = append(out, n)
		}
		return out
	}
	for _, test := range []struct {
		ips  []string
		want string
		fail bool
	}{
		{[]string{"192.168.1.2", "192.168.1.3"}, "192.168.1.2:45872", false},
		{[]string{"192.168.1.3"}, "192.168.1.3:45872", false},
		{[]string{"192.168.1.3", "192.168.1.4"}, "", true},
		{[]string{"8.8.8.8"}, "", true}, {nil, "", true},
	} {
		actual, err := chooseInterfaceAddress("192.168.1.2:45872", networks(test.ips...))
		if actual != test.want || (err != nil) != test.fail {
			t.Fatalf("actual=%s err=%v", actual, err)
		}
	}
}

func TestListenerRecoveryWakesAcceptAndKeepsSelectedBind(t *testing.T) {
	for _, changedIP := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_ip_after_adapter_recovery", true: "changed_ip"}[changedIP], func(t *testing.T) {
			listener, err := agent.NewListenSource("127.0.0.1:0", &tls.Config{})
			if err != nil {
				t.Fatal(err)
			}
			p := truststore.ConnectionProfile{Fingerprint: "same", ConnectionSettings: truststore.ConnectionSettings{Address: listener.Address(), Edge: "RIGHT"}}
			_, port, _ := net.SplitHostPort(p.Address)
			desired := p.Address
			if changedIP {
				desired = net.JoinHostPort("127.0.0.2", port)
			}
			var recoverable atomic.Bool
			ctx, cancel := context.WithCancel(context.Background())
			saved := 0
			s := &productListen{listener: listener, address: p.Address, fingerprint: p.Fingerprint, tls: &tls.Config{}, ctx: ctx, cancel: cancel, done: make(chan struct{}), load: func() (truststore.ConnectionProfile, error) { return p, nil }, save: func(before truststore.ConnectionProfile, address string) error {
				if before.Address != p.Address {
					t.Fatal("stale save")
				}
				p.Address = address
				saved++
				return nil
			}, resolve: func(string, int) (string, error) {
				if recoverable.Load() {
					return desired, nil
				}
				return "", errors.New("adapter down")
			}}
			go s.watch()
			defer s.Close()
			pending, stop := context.WithTimeout(ctx, 3*time.Second)
			defer stop()
			if conn, err := s.Next(pending); conn != nil || err == nil {
				t.Fatal("unavailable adapter accepted")
			}
			// A pending accept on the previously bound address is interrupted by the watcher.
			if conn, err := listener.Next(pending); conn != nil || err == nil {
				t.Fatal("invalidated listener accepted")
			}
			recoverable.Store(true)
			attempt, cancelAttempt := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancelAttempt()
			if conn, err := s.Next(attempt); conn != nil || err == nil {
				t.Fatal("canceled accept succeeded")
			}
			if s.address != desired || s.listener == listener || saved != map[bool]int{false: 0, true: 1}[changedIP] {
				t.Fatalf("address=%s saved=%d", s.address, saved)
			}
		})
	}
}
