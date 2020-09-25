package audiomixer

import (
	"context"
	"fmt"
	"github.com/notedit/gst"
	"log"
	"sync"
	"time"
)

const STATE_NOT_STARTED = 0
const STATE_PLAYING = 1
const STATE_PAUSED = 2
const STATE_STOPPED = 3

type DstPipe struct {
	name     string
	pipeline *gst.Pipeline
	mutex    *sync.Mutex
	sources  map[string]*Src

	adderEl   *gst.Element
	encoderEl *gst.Element
	sinkEl    *gst.Element

	eosChan chan bool

	state int
}

func CreateDstPipe(sources ...*Src) (p *DstPipe, err error) {
	name := "dst"
	pipeline, err := gst.PipelineNew(name)
	if err != nil {
		return
	}

	p = &DstPipe{
		name:     name,
		pipeline: pipeline,
		mutex:    &sync.Mutex{},
		sources:  make(map[string]*Src),
	}

	//capsFilterEl, err := gst.ElementFactoryMake("capsfilter", fmt.Sprintf("%s_capsfilter", name))
	p.adderEl, err = gst.ElementFactoryMake("adder", fmt.Sprintf("%s_adder", name))
	p.encoderEl, err = gst.ElementFactoryMake("opusenc", fmt.Sprintf("%s_encoder", name))
	p.sinkEl, err = gst.ElementFactoryMake("appsink", fmt.Sprintf("%s_sink", name))
	if err != nil {
		return
	}

	//pipeline.Add(capsFilterEl)
	pipeline.Add(p.adderEl)
	pipeline.Add(p.encoderEl)
	pipeline.Add(p.sinkEl)

	//capsFilterEl.Link(adderEl)
	p.adderEl.Link(p.encoderEl)
	p.encoderEl.Link(p.sinkEl)

	p = &DstPipe{
		name:     name,
		pipeline: pipeline,
		mutex:    &sync.Mutex{},
		sources:  make(map[string]*Src),
		eosChan:  make(chan bool),
		state:    STATE_PLAYING,
	}

	p.pipeline.SetState(gst.StatePlaying)

	// Link sources
	for _, src := range sources {
		err = p.LinkSrc(src)
		if err != nil {
			log.Print(err)
			continue
		}
	}

	log.Print("Sources linked")

	busMessagesCtx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Minute*1))
	go p.pollBusMessages(busMessagesCtx, p.pipeline.GetBus())

	return
}

func (p *DstPipe) LinkSrc(srcPipe *Src) error {

	p.pipeline.Add(&srcPipe.bin.Element)
	adderEl := p.pipeline.GetByName(fmt.Sprintf("%s_adder", p.name))

	template := adderEl.GetPadTemplate("sink_%u")
	requestPad := adderEl.GetRequestPad(template, "sink_%u", audioCaps)

	r := srcPipe.audioSinkPad.Link(requestPad)
	if int(r) != 0 {
		return fmt.Errorf("Ghostpad could not be linked to adder sinkpad")
	}

	p.mutex.Lock()
	p.sources[srcPipe.name] = srcPipe
	p.mutex.Unlock()

	return nil
}

func (p *DstPipe) AddSource(srcPipe *Src) {
	p.pipeline.Add(&srcPipe.bin.Element)
}

func (p *DstPipe) Run(parentCtx context.Context, sampleChan chan *gst.Sample) {

	p.pipeline.SetState(gst.StatePlaying)

	log.Print("Start pulling samples")
	// Start pulling samples
	sinkEl := p.pipeline.GetByName(fmt.Sprintf("%s_sink", p.name))
	log.Print(sinkEl)

	var sample *gst.Sample
	var err error
	for {
		select {
		case <-parentCtx.Done():
			log.Print("PARENT CTX Done")
			close(sampleChan)
			return
		case <-p.eosChan:
			log.Print("EOS RECEIVED")
			close(sampleChan)
			return
		default:
			sample, err = sinkEl.PullSample()
			if err != nil {
				log.Print(err)
				continue
			}
			sampleChan <- sample
		}
	}

}

func (p *DstPipe) Pause() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.pipeline.SetState(gst.StatePaused)
	p.state = STATE_PAUSED
}

func (p *DstPipe) Resume() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.pipeline.SetState(gst.StatePlaying)
	p.state = STATE_PLAYING
}

func (p *DstPipe) Stop(natural bool) {
	log.Print("STOPPING DST...", natural)

	//if !natural {
	//	p.mutex.Lock()
	//	p.stopChan = make(chan bool)
	//	p.mutex.Unlock()
	//
	//	go p.pipeline.SendEvent(gst.NewEosEvent())
	//
	//} else {
	//	p.mutex.Lock()
	//	defer p.mutex.Unlock()
	//
	//	if p.stopChan != nil {
	//		<-p.stopChan
	//	}
	//
	//	p.pipeline.SetState(gst.StateNull)
	//	p.pipeline = nil
	//	p.stopChan = nil
	//}

	log.Print("STOPPED")
}

//func (p *DstPipe) PullSample(ctx context.Context) *gst.Sample {
//
//	sinkEl := p.pipeline.GetByName(fmt.Sprintf("%s_sink", p.name))
//	if sinkEl == nil {
//		return nil
//	}
//
//	p.sampleChan = make(chan *gst.Sample)
//
//	go func() {
//		var sample *gst.Sample
//		var err error
//
//		for {
//			select {
//			case <-ctx.Done():
//				return
//			default:
//				if p.pipeline == nil {
//					close(p.sampleChan)
//					return
//				}
//				sample, err = sinkEl.PullSample()
//				if err != nil {
//					continue
//				}
//				p.sampleChan <- sample
//			}
//		}
//	}()
//
//	return p.sampleChan
//}

func (p *DstPipe) GetBus() *gst.Bus {
	return p.pipeline.GetBus()
}

func (p *DstPipe) SetState(state gst.StateOptions) {
	p.pipeline.SetState(state)
}

func (p *DstPipe) GetState() int {
	return p.state
}

func (p *DstPipe) pollBusMessages(ctx context.Context, bus *gst.Bus) {
	if bus == nil {
		log.Print("Bus is nil")
		return
	}

	log.Print("Pulling bus-messages")

	for {
		select {
		case <-ctx.Done():
			log.Print("Stopping pulling bus-messages")
			return
		default:
			log.Print("Waiting for bus-message")
			message := bus.Pull(gst.MessageError | gst.MessageWarning | gst.MessageEos)
			log.Print("Bus-message received")
			log.Printf("Message: %s", message.GetName())

			if message.GetStructure().C != nil {
				log.Print(message.GetStructure().ToString())
			}

			if message.GetType() == gst.MessageEos {
				//p.Stop(true)
				p.eosChan <- true
				return
			}
		}
	}
}
