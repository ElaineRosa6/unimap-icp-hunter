package web

import (
	"github.com/unimap-icp-hunter/project/internal/distributed"
)

// DistributedState holds the distributed node registry and task queue.
type DistributedState struct {
	NodeRegistry  *distributed.Registry
	NodeTaskQueue *distributed.TaskQueue
}
