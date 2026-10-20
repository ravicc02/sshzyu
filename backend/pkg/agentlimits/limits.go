// Package agentlimits defines the bounded storage and image result envelope.
// Agent outputs are not constrained by the public batch JSONL scanner limits.
package agentlimits

const (
	GeneratedImageBytes       = 40 << 20
	ImageResponseBytes        = 64 << 20
	ImagePixels         int64 = 40000000
	ImageSide                 = 8192
	UserStorageBytes    int64 = 1 << 30
)
