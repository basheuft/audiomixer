package audiomixer

import "C"
import (
	"fmt"
	"github.com/notedit/gst"
	"log"
	"strings"
	"time"
)

type EOSFunc func(s *Source)

type Source struct {
	name     string
	bin      *gst.Bin
	pipeline *gst.Pipeline

	element    *gst.Element
	convertEl  *gst.Element
	resampleEl *gst.Element
	volumeEl   *gst.Element

	mixerSinkPad *gst.Pad

	onEOS         EOSFunc
	tearDownDone chan bool
}

func (s *Source) ChangeURI(uri string) {
	s.element.SetObject("uri", uri)
}

func (s *Source) Position() (time.Duration, error) {
	return s.element.QueryPosition()
}

func (s *Source) Duration() (time.Duration, error) {
	return s.element.QueryDuration()
}

func (s *Source) Seek(dst time.Duration) bool {
	return s.element.Seek(dst)
}

func (s *Source) SetVolume(volume float64) {
	s.volumeEl.SetObject("volume", volume)
}

func (s *Source) SyncStateWithParent() {
	s.element.SyncStateWithParent()
}

func (s *Source) Teardown() {
	log.Print("TEARING DOWN")
	go func() {
		s.bin.SetState(gst.StateNull)
		s.pipeline.Remove(&s.bin.Element)
		s.tearDownDone <- true
	}()
}

func (s *Source) Stop() {
}

func CreateSource(name, uri string, pipeline *gst.Pipeline, volume float64, onEOS EOSFunc) (s *Source, err error) {
	binName := fmt.Sprintf("bin_%s", name)

	// Check if a bin with this name already exists
	r := pipeline.GetByName(binName)
	if r != nil {
		err = fmt.Errorf("A bin with name %s already exists in pipeline", binName)
		return
	}

	bin := gst.BinNew(binName)

	sourceEl, err := gst.ElementFactoryMake("uridecodebin", fmt.Sprintf("%s_source", name))
	convertEl, err := gst.ElementFactoryMake("audioconvert", fmt.Sprintf("%s_convert", name))
	resampleEl, err := gst.ElementFactoryMake("audioresample", fmt.Sprintf("%s_resample", name))
	volumeEl, err := gst.ElementFactoryMake("volume", fmt.Sprintf("%s_volume", name))
	if err != nil {
		return
	}

	volumeEl.SetObject("volume", volume)

	s = &Source{
		name:          name,
		bin:           bin,
		pipeline:      pipeline,
		element:       sourceEl,
		convertEl:     convertEl,
		resampleEl:    resampleEl,
		volumeEl:      volumeEl,
		onEOS:         onEOS,
		tearDownDone: make(chan bool),
	}

	bin.Add(sourceEl)
	bin.Add(convertEl)
	bin.Add(resampleEl)
	bin.Add(volumeEl)

	sourceEl.Link(convertEl)
	convertEl.Link(resampleEl)
	resampleEl.Link(volumeEl)

	sourceEl.SetObject("uri", uri)
	sourceEl.SetObject("caps", gst.CapsFromString("audio/x-raw"))

	sourceEl.SetPadAddedCallback(func(callback gst.Callback, args ...interface{}) {
		pad := args[0].(*gst.Pad)
		capstr := pad.GetCurrentCaps().ToString()

		if strings.HasPrefix(capstr, "audio") {
			convertSinkPad := convertEl.GetStaticPad("sink")
			pad.Link(convertSinkPad)

			volumeSrcPad := volumeEl.GetStaticPad("src")

			ghostPad := bin.NewGhostPad("src", volumeSrcPad)

			padTemplate := mixer.GetPadTemplate("sink_%u")
			s.mixerSinkPad = mixer.GetRequestPad(padTemplate, "sink_%u", pad.GetCurrentCaps())

			bin.AddPad(ghostPad)
			ghostPad.Link(s.mixerSinkPad)

			//s.mixerSinkPad = sinkPad
			ghostPad.SetProbeCallback(gst.PAD_PROBE_TYPE_EVENT_DOWNSTREAM, func(callback gst.PadCallback, args ...interface{}) {
				eventName := args[1].(string)
				if eventName == "eos" {
					// End of stream, teardown and start callback
					s.Teardown()
					defer s.onEOS(s)
					return
				} else {
					//log.Printf("PAD PROBE EVENT: %s", eventName)
				}
			})
		}
	})

	bin.SetState(gst.StatePlaying)
	pipeline.Add(&bin.Element)

	return
}
