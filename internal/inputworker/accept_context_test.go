package inputworker

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestAgentAcceptRetainsSingleUseBoundAndServiceCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, stop := context.WithCancel(context.Background())
		defer stop()
		service, cancelService := agentAcceptContext(parent, true)
		defer cancelService()
		single, cancelSingle := agentAcceptContext(parent, false)
		defer cancelSingle()
		<-single.Done()
		if single.Err() != context.DeadlineExceeded || service.Err() != nil {
			t.Fatal(single.Err(), service.Err())
		}
		stop()
		<-service.Done()
		if service.Err() != context.Canceled {
			t.Fatal(service.Err())
		}
	})
}

func TestServiceAcceptCannotExtendCallerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		service, cancel := agentAcceptContext(parent, true)
		defer cancel()
		<-service.Done()
		if service.Err() != context.DeadlineExceeded {
			t.Fatal(service.Err())
		}
	})
}
