package engines

import "context"

// Syncer is the interface all sync engines must implement.
type Syncer interface {
	Name() string
	Run(ctx context.Context) error
}
