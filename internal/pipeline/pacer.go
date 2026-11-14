package pipeline

import (
	"context"
	"time"
)

const staticLead = 10 * time.Second

type pacer struct {
	start  time.Time
	media  time.Duration
	active bool
}

func (p *pacer) add(seconds float64) {
	if seconds > 0 {
		p.media += time.Duration(seconds * float64(time.Second))
	}
}

func (p *pacer) hold(ctx context.Context) {
	if !p.active {
		return
	}
	if p.start.IsZero() {
		p.start = time.Now()
	}
	if ahead := p.media - time.Since(p.start) - staticLead; ahead > 0 {
		wait(ctx, ahead)
	}
}
