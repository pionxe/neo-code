package cli

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestGatewayIdleShutdownControllerCancelsAfterIdleTimeout(t *testing.T) {
	var cancelCount atomic.Int32
	controller := newGatewayIdleShutdownController(nil, func() {
		cancelCount.Add(1)
	})
	controller.idleTimeout = 30 * time.Millisecond
	t.Cleanup(controller.close)

	controller.observe(0)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cancelCount.Load() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected cancel to be called after idle timeout")
}

func TestGatewayIdleShutdownControllerCancelsTimerWhenConnectionRecovers(t *testing.T) {
	var cancelCount atomic.Int32
	controller := newGatewayIdleShutdownController(nil, func() {
		cancelCount.Add(1)
	})
	controller.idleTimeout = 80 * time.Millisecond
	t.Cleanup(controller.close)

	controller.observe(0)
	time.Sleep(20 * time.Millisecond)
	controller.observe(1)
	time.Sleep(120 * time.Millisecond)

	if cancelCount.Load() != 0 {
		t.Fatalf("expected idle timer to be cancelled when connection recovers")
	}
}
