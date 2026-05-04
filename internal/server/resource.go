package server

// ResourceUsage is a per-process resource snapshot the GUI displays
// next to each running server
type ResourceUsage struct {
	PID        int
	RAMBytes   uint64
	CPUPercent float64
}

type ResourceSampler interface {
	Sample(pid int) (ResourceUsage, error)
}

type NoopSampler struct{}

func (NoopSampler) Sample(pid int) (ResourceUsage, error) {
	return ResourceUsage{PID: pid}, nil
}

var _ ResourceSampler = NoopSampler{}
