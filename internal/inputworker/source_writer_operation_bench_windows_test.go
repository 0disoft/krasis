//go:build windows

package inputworker

import (
	"context"
	"testing"
)

func BenchmarkSourceWriterOwnerOperation(b *testing.B) {
	owner, cancelOwner := context.WithCancel(context.Background())
	defer cancelOwner()
	w := &sourceWriter{ctx: owner}
	b.ReportAllocs()
	for b.Loop() {
		_, cancel := w.operation(nil, false)
		cancel()
	}
}
