package audiomixer

import "github.com/notedit/gst"

type Pipe struct {
	pipeline *gst.Pipeline
}

func (p *Pipe) GetBus() *gst.Bus {
	return p.pipeline.GetBus()
}

func (p *Pipe) SetState(state gst.StateOptions) {
	p.pipeline.SetState(state)
}
