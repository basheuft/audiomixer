package audiomixer

import (
	"fmt"
	"github.com/notedit/gst"
	"log"
	"sync"
)

const STATE_NOT_STARTED = 0
const STATE_PLAYING = 1
const STATE_PAUSED = 2
const STATE_STOPPED = 3

type DstPipe struct {
	Pipe
	name string

	mutex *sync.Mutex
	state int

	stopChan chan bool

	sampleChan chan *gst.Sample
}

func CreateDstPipe() (pipe *DstPipe, err error) {
	name := "dst"
	pipeline, err := gst.PipelineNew(name)
	if err != nil {
		return
	}

	//capsFilterEl, err := gst.ElementFactoryMake("capsfilter", fmt.Sprintf("%s_capsfilter", name))
	adderEl, err := gst.ElementFactoryMake("adder", fmt.Sprintf("%s_adder", name))
	encoderEl, err := gst.ElementFactoryMake("opusenc", fmt.Sprintf("%s_encoder", name))
	sinkEl, err := gst.ElementFactoryMake("appsink", fmt.Sprintf("%s_sink", name))
	if err != nil {
		return
	}

	//pipeline.Add(capsFilterEl)
	pipeline.Add(adderEl)
	pipeline.Add(encoderEl)
	pipeline.Add(sinkEl)

	//capsFilterEl.Link(adderEl)
	adderEl.Link(encoderEl)
	encoderEl.Link(sinkEl)

	pipe = &DstPipe{
		name: name,
		Pipe: Pipe{pipeline: pipeline},
		mutex: &sync.Mutex{},
		state: STATE_NOT_STARTED,
	}

	bus := pipeline.GetBus()
	go pipe.pollBusMessages(bus)

	return
}

func (p *DstPipe) LinkSrc(srcPipe *SrcPipe) (err error) {
	//capsFilterEl.SetObject("caps", gst.CapsFromString("audio/x-raw"))
	//channel := fmt.Sprintf("%s_%s", srcPipe.name, p.name)
	adderEl := p.pipeline.GetByName(fmt.Sprintf("%s_adder", p.name))

	template := adderEl.GetPadTemplate("sink_%u")
	requestPad := adderEl.GetRequestPad(template, "sink_%u", audioCaps)

	srcPipe.audioSinkPad.Link(requestPad)

	return
}

func (p *DstPipe) AddSource(srcPipe *SrcPipe) {
	p.pipeline.Add(&srcPipe.bin.Element)
}

func (p *DstPipe) Start() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.state = STATE_PLAYING
	p.pipeline.SetState(gst.StatePlaying)
}

func (p *DstPipe) Pause() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.state = STATE_PAUSED
	p.pipeline.SetState(gst.StatePaused)
}

func (p *DstPipe) Resume() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.state = STATE_PLAYING
	p.pipeline.SetState(gst.StatePlaying)
}

func (p *DstPipe) GetState() int {
	return p.state
}

func (p *DstPipe) Stop(natural bool) {
	log.Print("STOPPING DST...", natural)
	if p.state == STATE_STOPPED || p.pipeline == nil {
		log.Print("ALREADY STOPPED..")
		return
	}

	if !natural {
		p.mutex.Lock()
		p.stopChan = make(chan bool)
		p.mutex.Unlock()

		go p.pipeline.SendEvent(gst.NewEosEvent())

	} else {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.stopChan != nil {
			<-p.stopChan
		}

		p.pipeline.SetState(gst.StateNull)
		p.pipeline = nil
		p.stopChan = nil
	}

	p.state = STATE_STOPPED

	log.Print("STOPPED")
}

func (p *DstPipe) IsPlaying() bool {
	return p.state == STATE_PLAYING
}

func (p *DstPipe) IsPaused() bool {
	return p.state == STATE_PAUSED
}

func (p *DstPipe) PullSample() chan *gst.Sample {

	sinkEl := p.pipeline.GetByName(fmt.Sprintf("%s_sink", p.name))
	if sinkEl == nil {
		return nil
	}

	p.sampleChan = make(chan *gst.Sample)

	go func() {
		var sample *gst.Sample
		var err error
		for {
			if p.pipeline == nil {
				close(p.sampleChan)
				return
			}
			sample, err = sinkEl.PullSample()
			if err != nil {
				continue
			}
			p.sampleChan <- sample
		}
	}()

	return p.sampleChan
}

func (p *DstPipe) pollBusMessages(bus *gst.Bus) {
	for {
		message := bus.Pull(gst.MessageError | gst.MessageWarning | gst.MessageEos)
		log.Printf("Message: %s", message.GetName())

		if message.GetStructure().C != nil {
			log.Print(message.GetStructure().ToString())
		}

		if message.GetType() == gst.MessageEos {
			p.Stop(true)
			if p.stopChan != nil {
				p.stopChan <- true
			}
			return
		}
	}
}