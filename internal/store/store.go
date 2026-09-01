package store

import "context"

// BitStore is the storage contract for the scalable bloom filter. Implementations
// must be safe for concurrent use by multiple goroutines.
type BitStore interface {
	// SetBits sets the bits at the given offsets to 1 in the bitmap stored at key.
	SetBits(ctx context.Context, key string, offsets []uint64) error

	// GetBits returns the value of each bit at the given offsets in the bitmap at key.
	// The returned slice has the same length and order as offsets.
	GetBits(ctx context.Context, key string, offsets []uint64) ([]bool, error)

	// HGetAll returns all field-value pairs in the hash stored at key.
	HGetAll(ctx context.Context, key string) (map[string]string, error)

	// HSet sets the given field-value pairs in the hash stored at key.
	HSet(ctx context.Context, key string, fields map[string]string) error

	// Incr atomically increments the integer value at key by one and returns the
	// new value. A non-existent key is treated as 0 before incrementing.
	Incr(ctx context.Context, key string) (int64, error)

	// SAdd adds members to the set stored at key.
	SAdd(ctx context.Context, key string, members ...string) error

	// SMembers returns all members of the set stored at key.
	SMembers(ctx context.Context, key string) ([]string, error)

	// Del removes the specified keys and their associated values.
	Del(ctx context.Context, keys ...string) error

	// AppendStage atomically appends a new stage to a filter only if the current
	// stage_count equals expected. It writes newStageFields to newStageMetaKey and
	// bumps stage_count in metaKey. Returns the new stage_count. If stage_count
	// has already advanced (lost race), it returns the current count without
	// modifying anything.
	AppendStage(ctx context.Context, metaKey string, expected int64, newStageFields map[string]string, newStageMetaKey string) (int64, error)
}
