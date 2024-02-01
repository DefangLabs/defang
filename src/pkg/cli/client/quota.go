package client

// These serve mostly to pevent fat-finger errors in the CLI or Compose files
const (
	maxCpus       = 8 * 2
	maxGpus       = 8 * 1
	maxMemoryMiB  = 8 * 8192
	maxReplicas   = 8 * 2
	maxServices   = 8 * 5
	maxShmSizeMiB = 8 * 30720
)
