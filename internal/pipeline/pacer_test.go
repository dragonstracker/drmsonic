package pipeline

import (
	"context"
	"testing"
	"time"
)

func TestPacerHoldsWhenAheadOfRealtime(t *testing.T) {
	p := pacer{active: true}
	p.hold(context.Background())
	p.add(staticLead.Seconds() + 0.3)

	start := time.Now()
	p.hold(context.Background())
	if d := time.Since(start); d < 200*time.Millisecond || d > 2*time.Second {
		t.Fatalf("held %s, want about 300ms", d)
	}
}

func TestPacerIdleWithinLead(t *testing.T) {
	p := pacer{active: true}
	p.add(staticLead.Seconds() - 1)

	start := time.Now()
	p.hold(context.Background())
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("held %s inside the lead window", d)
	}
}

func TestPacerInactiveNeverHolds(t *testing.T) {
	p := pacer{}
	p.add(3600)

	start := time.Now()
	p.hold(context.Background())
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("inactive pacer held %s", d)
	}
}
