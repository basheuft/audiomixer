package audiomixer

import (
	"fmt"
	"github.com/notedit/gst"
	"log"
	"sync"
	"time"
)

type SrcPipe struct {
	bin          *gst.Bin
	name         string
	dst          *DstPipe
	audioSinkPad *gst.Pad

	pauseChan chan bool
	stopChan chan bool

	mutex *sync.Mutex
}

func CreateSourcePipe(name, uri string, dst *DstPipe) (pipe *SrcPipe, err error) {
	bin := gst.BinNew(name)

	playbinEl, err := gst.ElementFactoryMake("uridecodebin3", fmt.Sprintf("%s_decode", name))
	convertEl, err := gst.ElementFactoryMake("audioconvert", fmt.Sprintf("%s_convert", name))
	resampleEl, err := gst.ElementFactoryMake("audioresample", fmt.Sprintf("%s_resample", name))
	volumeEl, err := gst.ElementFactoryMake("volume", fmt.Sprintf("%s_volume", name))
	if err != nil {
		return
	}

	bin.Add(playbinEl)
	bin.Add(convertEl)
	bin.Add(resampleEl)
	//pipeline.Add(capsFilterEl)
	bin.Add(volumeEl)

	playbinEl.Link(convertEl)
	convertEl.Link(resampleEl)
	resampleEl.Link(volumeEl)
	//capsFilterEl.Link(volumeEl)

	playbinEl.SetObject("caps", gst.CapsFromString("audio/x-raw"))
	playbinEl.SetObject("uri", uri)
	playbinEl.SetObject("buffer-size", 32)

	pipe = &SrcPipe{
		bin:  bin,
		name: name,
		dst:  dst,
		mutex: &sync.Mutex{},
	}

	volumeSrcPad := volumeEl.GetStaticPad("src")
	pipe.audioSinkPad = bin.NewGhostPad("src", volumeSrcPad)
	bin.AddPad(pipe.audioSinkPad)

	playbinEl.SetPadAddedCallback(func(callback gst.Callback, args ...interface{}) {
		pad := args[0].(*gst.Pad)
		log.Print("PAD ADDED", pad.Name())
		sinkPad := convertEl.GetStaticPad("sink")

		pad.Link(sinkPad)

		log.Printf("Linked uridecodebin3 (%s) to audioconvert (%s)", pad.Name(), sinkPad.Name())
	})

	pipe.audioSinkPad.SetProbeCallback(gst.PAD_PROBE_TYPE_EVENT_DOWNSTREAM, func(callback gst.PadCallback, args ...interface{}) int {
		eventName := args[1].(string)
		if eventName == "eos" {
			if pipe.stopChan != nil {
				pipe.stopChan <- true
			}
			return gst.PAD_PROBE_REMOVE
		}

		return gst.PAD_PROBE_OK
	})

	dst.AddSource(pipe)

	return
}

func (p *SrcPipe) Start() {
	if p.bin == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.dst.pipeline.SetState(gst.StatePlaying)
	p.bin.SetState(gst.StatePlaying)

	p.dst.Start()
}

func (p *SrcPipe) Stop() {
	log.Print("STOPPING SRC")
	if p.bin == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.stopChan = make(chan bool)
	p.bin.SendEvent(gst.NewEosEvent())
	<-p.stopChan
	p.bin.SetState(gst.StateNull)
	p.bin = nil
	p.stopChan = nil
}

func (p *SrcPipe) SetVolume(vol int) {
	if p.bin == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	volumeEl := p.bin.GetByName(fmt.Sprintf("%s_volume", p.name))
	if volumeEl == nil {
		return
	}

	volume := float64(vol) / 100
	if volume > 1.0 || volume < 0.0 {
		return
	}

	volumeEl.SetObject("volume", volume)
}

func (p *SrcPipe) Seek(duration time.Duration) {
	if p.bin == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	decodeEl := p.bin.GetByName(fmt.Sprintf("%s_decode", p.name))
	total, err := decodeEl.QueryDuration()
	if err != nil {
		return
	}

	if duration > total {
		return
	}

	decodeEl.Seek(duration)
}

func (p *SrcPipe) Position() (time.Duration, error) {
	if p.bin == nil {
		return 0, nil
	}

	decodeEl := p.bin.GetByName(fmt.Sprintf("%s_decode", p.name))
	if decodeEl == nil {
		return 0, nil
	}
	return decodeEl.QueryPosition()
}

func (p *SrcPipe) Duration() (time.Duration, error) {
	if p.bin == nil {
		return 0, nil
	}

	decodeEl := p.bin.GetByName(fmt.Sprintf("%s_decode", p.name))
	if decodeEl == nil {
		return 0, nil
	}
	return decodeEl.QueryDuration()
}

func (p *SrcPipe) BlockProbe() {
	log.Print("PAUSE CALLED")
	p.audioSinkPad.SetProbeCallback(gst.PAD_PROBE_TYPE_BLOCK_DOWNSTREAM, func(callback gst.PadCallback, args ...interface{}) int {
		log.Print("PROBE CALLBACK")
		if p.pauseChan == nil {
			p.pauseChan = make(chan bool)
		}
		<-p.pauseChan

		close(p.pauseChan)
		p.pauseChan = nil

		return gst.PAD_PROBE_REMOVE
	})
}

func (p *SrcPipe) UnblockProbe() {
	if p.pauseChan != nil {
		p.pauseChan <- true
	}
}
