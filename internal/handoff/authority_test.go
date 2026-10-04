package handoff

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestAuthorityDetailStaysWithMatchingInputAndCannotOverwrite(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(p, "installed")
	RecordAuthority(p, AuthorityServiceStatus, time.Second) // preparation is not input
	x, complete := Input(ctx, "receiver")
	RecordAuthority(x, AuthorityLock, 0)
	RecordAuthority(x, AuthorityPipeOwner, 3*time.Microsecond)
	RecordAuthority(x, AuthorityServiceStatus, 15*time.Millisecond)
	RecordAuthority(x, AuthorityServiceStatus, time.Second) // cleanup cannot replace
	complete(nil)
	RecordAuthority(x, AuthorityServiceStatus, 2*time.Second) // copy already saved
	x, complete = Input(ctx, "receiver")
	RecordAuthority(x, AuthorityPipeOwner, time.Microsecond)
	RecordAuthority(x, AuthorityServiceStatus, 7*time.Millisecond)
	x.Value(sampleKey{}).(*sample).started = time.Now().Add(-30 * time.Millisecond)
	complete(nil)
	x, complete = Input(ctx, "receiver")
	RecordAuthority(x, AuthorityServiceStatus, time.Second)
	x.Value(sampleKey{}).(*sample).started = time.Now()
	complete(nil)
	Finish(p, "ok")
	s := (*out)[0]
	if s.Preparation.AuthorityMeasured != 0 || s.First.AuthorityMeasured != 7 || s.First.AuthorityMicros != [3]uint32{0, 3, 15000} || s.Worst.AuthorityMeasured != 6 || s.Worst.AuthorityMicros != [3]uint32{0, 1, 7000} {
		t.Fatal("authority spans mixed between input or preparation", s)
	}
	for _, mutate := range []func(*Vector){
		func(v *Vector) { v.AuthorityMeasured = 8 },
		func(v *Vector) { v.AuthorityMeasured = 0 },
		func(v *Vector) { v.AuthorityMicros[2] = uint32(Window.Microseconds() + 1) },
	} {
		bad := s
		mutate(&bad.First)
		if bad.Valid() {
			t.Fatal("invalid authority detail accepted")
		}
	}
}

func TestAuthorityDetailLegacyAbsentAndClamp(t *testing.T) {
	var v Vector
	if err := json.Unmarshal([]byte(`{"outcome":"ok"}`), &v); err != nil || !ValidAuthorityDetail(v.AuthorityMicros, v.AuthorityMeasured) {
		t.Fatal("legacy absence rejected")
	}
	RecordAuthority(nil, AuthorityLock, time.Second)
	RecordAuthority(context.Background(), AuthorityLock, time.Second)
	_, ctx, _ := collected(t)
	Begin(ctx, "receiver", "session", "epoch", time.Now())
	x, complete := Input(ctx, "receiver")
	RecordAuthority(x, AuthorityLock, -time.Second)
	RecordAuthority(x, AuthorityServiceStatus, Window+time.Second)
	RecordAuthority(x, AuthorityStageCount, time.Second)
	v = x.Value(sampleKey{}).(*sample).vector
	if v.AuthorityMeasured != 5 || v.AuthorityMicros != [3]uint32{0, 0, uint32(Window.Microseconds())} || !ValidAuthorityDetail(v.AuthorityMicros, v.AuthorityMeasured) {
		t.Fatal("presence or clamp changed", v)
	}
	complete(nil)
}
