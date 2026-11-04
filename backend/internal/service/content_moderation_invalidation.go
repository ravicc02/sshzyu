package service

import "sync/atomic"

var contentModerationSettingsVersion atomic.Uint64

// InvalidateContentModerationRuntime expires immutable snapshots without taking
// a gateway lock. The existing refresh worker serializes DB reads off that path.
func InvalidateContentModerationRuntime() { contentModerationSettingsVersion.Add(1) }
