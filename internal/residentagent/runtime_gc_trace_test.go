package residentagent

import (
	"bytes"
	"runtime/trace"
	"testing"
)

func TestGCSamplerTraceSeparatesRuntimeAndOSReads(t *testing.T) {
	var output bytes.Buffer
	if err := trace.Start(&output); err != nil {
		t.Fatal(err)
	}
	tracing := true
	defer func() {
		if tracing {
			trace.Stop()
		}
	}()
	read := newGCSampler()
	sample := read()
	trace.Stop()
	tracing = false
	if sample.at.IsZero() || sample.known == 0 {
		t.Fatal("trace changed metric availability", sample.known)
	}
	for _, name := range []string{"krasis.runtime.gc_metrics", "krasis.runtime.os_counters"} {
		if !bytes.Contains(output.Bytes(), []byte(name)) {
			t.Error("missing sampler phase", name)
		}
	}
}
