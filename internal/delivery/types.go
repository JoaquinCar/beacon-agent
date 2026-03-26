package delivery

import (
	"context"

	"github.com/joako/beacon/internal/briefing"
)

// Sender delivers a briefing to an external channel.
// Implementations must check config.DryRun before any network call.
type Sender interface {
	Send(ctx context.Context, b briefing.Briefing) error
}
