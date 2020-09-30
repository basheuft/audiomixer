package audiomixer

import (
	"fmt"
	"github.com/notedit/gst"
	"log"
	"sync"
	"time"
)

type Src struct {
	bin          *gst.Bin
	name         string
	dst          *DstPipe
	audioSinkPad *gst.Pad

	mutex *sync.Mutex

	decodeEl, convertEl, resampleEl, volumeEl *gst.Element
}

func CreateSource(name, uri string, onEOS func()) (pipe *Src, err error) {
	bin := gst.BinNew(name)

	decodeEl, err := gst.ElementFactoryMake("uridecodebin3", fmt.Sprintf("%s_decode", name))
	convertEl, err := gst.ElementFactoryMake("audioconvert", fmt.Sprintf("%s_convert", name))
	resampleEl, err := gst.ElementFactoryMake("audioresample", fmt.Sprintf("%s_resample", name))
	volumeEl, err := gst.ElementFactoryMake("volume", fmt.Sprintf("%s_volume", name))
	if err != nil {
		return
	}

	bin.Add(decodeEl)
	bin.Add(convertEl)
	bin.Add(resampleEl)
	bin.Add(volumeEl)

	decodeEl.Link(convertEl)
	convertEl.Link(resampleEl)
	resampleEl.Link(volumeEl)

	decodeEl.SetObject("caps", gst.CapsFromString("audio/x-raw"))
	decodeEl.SetObject("uri", uri)
	decodeEl.SetObject("buffer-size", 32)

	pipe = &Src{
		bin:  bin,
		name: name,
		mutex: &sync.Mutex{},

		decodeEl: decodeEl,
		convertEl: convertEl,
		resampleEl: resampleEl,
		volumeEl: volumeEl,
	}

	volumeSrcPad := volumeEl.GetStaticPad("src")
	pipe.audioSinkPad = bin.NewGhostPad("src", volumeSrcPad)
	bin.AddPad(pipe.audioSinkPad)

	decodeEl.SetPadAddedCallback(func(callback gst.Callback, args ...interface{}) {
		pad := args[0].(*gst.Pad)
		log.Print("PAD ADDED", pad.Name())
		sinkPad := convertEl.GetStaticPad("sink")

		pad.Link(sinkPad)

		log.Printf("Linked uridecodebin3 (%s) to audioconvert (%s)", pad.Name(), sinkPad.Name())
	})

	pipe.audioSinkPad.SetProbeCallback(gst.PAD_PROBE_TYPE_EVENT_DOWNSTREAM, func(callback gst.PadCallback, args ...interface{}) int {
		eventName := args[1].(string)
		if eventName == "eos" {
			go func() {
				// Cleanup
				pipe.Cleanup()
				onEOS()
			}()
			return gst.PAD_PROBE_REMOVE
		}

		return gst.PAD_PROBE_OK
	})

	bin.SetState(gst.StatePlaying)

	return
}

func (p *Src) SetVolume(vol int) {
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

func (p *Src) Seek(duration time.Duration) {
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

func (p *Src) Position() (time.Duration, error) {
	if p.bin == nil {
		return 0, nil
	}

	decodeEl := p.bin.GetByName(fmt.Sprintf("%s_decode", p.name))
	if decodeEl == nil {
		return 0, nil
	}
	return decodeEl.QueryPosition()
}

func (p *Src) Duration() (time.Duration, error) {
	if p.bin == nil {
		return 0, nil
	}

	decodeEl := p.bin.GetByName(fmt.Sprintf("%s_decode", p.name))
	if decodeEl == nil {
		return 0, nil
	}
	return decodeEl.QueryDuration()
}

func (p *Src) Cleanup() {
	log.Print("SRC PIPE ENDED")
}

//func (p *SrcPipe) BlockProbe() {
//	log.Print("PAUSE CALLED")
//	p.audioSinkPad.SetProbeCallback(gst.PAD_PROBE_TYPE_BLOCK_DOWNSTREAM, func(callback gst.PadCallback, args ...interface{}) int {
//		log.Print("PROBE CALLBACK")
//		if p.pauseChan == nil {
//			p.pauseChan = make(chan bool)
//		}
//		<-p.pauseChan
//
//		close(p.pauseChan)
//		p.pauseChan = nil
//
//		return gst.PAD_PROBE_REMOVE
//	})
//}
//
//func (p *SrcPipe) UnblockProbe() {
//	if p.pauseChan != nil {
//		p.pauseChan <- true
//	}
//}
