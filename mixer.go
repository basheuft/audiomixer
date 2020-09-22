package audiomixer

import (
	"fmt"
	"github.com/notedit/gst"
	"log"
	"sync"
	"time"
)

var pipeline *gst.Pipeline
var mixer *gst.Element
var sink *gst.Element

var mutex = &sync.Mutex{}
var MainSource *Source
var otherSources = make(map[string]*Source)

//func main() {
//	log.SetFlags(log.Ltime | log.Lshortfile)
//
//	//uri := "file:///home/bas/Documents/Projecten/rudi/jingles/sigaar-uit-eigen-kist.ogg"
//	uri := "https://italo.italo.nu/"
//
//	ended, err := PlayMain(uri)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	time.Sleep(time.Second * 5)
//	anotherEnded, err := PlayAnother("file:///home/bas/Documents/Projecten/rudi/jingles/badkamer-volgescheten.ogg", "")
//	if err != nil {
//		log.Print(err)
//	}
//
//	time.Sleep(time.Millisecond * 300)
//	anotherEnded, err = PlayAnother("file:///home/bas/Documents/Projecten/rudi/jingles/badkamer-volgescheten.ogg", "")
//	if err != nil {
//		log.Print(err)
//	}
//
//	time.Sleep(time.Millisecond * 300)
//	anotherEnded, err = PlayAnother("file:///home/bas/Documents/Projecten/rudi/jingles/badkamer-volgescheten.ogg", "")
//	if err != nil {
//		log.Print(err)
//	}
//	time.Sleep(time.Millisecond * 300)
//	anotherEnded, err = PlayAnother("file:///home/bas/Documents/Projecten/rudi/jingles/badkamer-volgescheten.ogg", "")
//	if err != nil {
//		log.Print(err)
//	}
//	time.Sleep(time.Millisecond * 300)
//	anotherEnded, err = PlayAnother("file:///home/bas/Documents/Projecten/rudi/jingles/badkamer-volgescheten.ogg", "")
//	if err != nil {
//		log.Print(err)
//	}
//
//	go func() {
//		<-anotherEnded
//		log.Print("ANOTHER ENDED")
//	}()
//
//	<-ended
//	log.Print("Pipeline ended")
//}

func PullSinkSample() (sampleChan chan *gst.Sample) {
	sampleChan = make(chan *gst.Sample)
	go func() {
		for {
			if sink == nil {
				continue
			}
			sample, _ := sink.PullSample()
			sampleChan <- sample
		}
	}()

	return sampleChan
}

func PlayMain(uri string) (ended chan bool, err error) {
	ended = make(chan bool)
	if pipeline != nil {
		err = fmt.Errorf("[AUDIOMIXER] pipeline already created")
		return
	}

	err = bootstrapPipeline()
	if err != nil {
		return
	}

	// Create and play source
	MainSource, err = CreateSource(
		"main",
		uri,
		pipeline,
		0.2,
		func (s *Source) {
			MainSource = nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	pipeline.SetState(gst.StatePlaying)

	bus := pipeline.GetBus()
	go func() {
		for {
			message := bus.Pull(gst.MessageError | gst.MessageWarning | gst.MessageEos)
			log.Printf("Message: %s", message.GetName())

			if message.GetStructure().C != nil {
				log.Print(message.GetStructure().ToString())
			}

			if message.GetType() == gst.MessageEos {
				log.Print("RECEIVED EOS")
				ended <- true

				pipeline = nil
				mixer = nil
				sink = nil

				break
			}
		}
	}()

	return
}

func bootstrapPipeline() (err error) {
	if pipeline != nil {
		log.Print("[AUDIOMIXER] Pipeline already bootstrapped")
		return
	}

	log.Print("[AUDIOMIXER] Bootstrapping pipeline..")

	mutex.Lock()
	defer mutex.Unlock()

	pipeline, err = gst.PipelineNew("pipe")
	if err != nil {
		log.Fatal(err)
	}

	// Create pipeline elements
	mixer, err = gst.ElementFactoryMake("adder", "mixer")
	encoder, err := gst.ElementFactoryMake("opusenc", "encoder")
	sink, err = gst.ElementFactoryMake("appsink", "sink")
	if err != nil {
		log.Fatal(err)
	}

	pipeline.Add(mixer)
	pipeline.Add(encoder)
	pipeline.Add(sink)

	mixer.Link(encoder)
	encoder.Link(sink)

	log.Print(mixer.Name())

	return
}

func PlayAnother(uri, elementName string) (ended chan bool, err error) {
	ended = make(chan bool)
	if pipeline == nil {
		err = bootstrapPipeline()
		if err != nil {
			return
		}
	}

	// Generate unique element-name if not given
	if len(elementName) < 1 {
		elementName = generateUniqueElementName()
	}

	log.Printf("[AUDIOMIXER] play another. elementname: %s, uri: %s", elementName, uri)

	src, err := CreateSource(
		elementName,
		uri,
		pipeline,
		1.0,
		func(s *Source) {
			// Remove from otherSources
			mutex.Lock()
			delete(otherSources, elementName)
			mutex.Unlock()

			ended <- true
		},
	)
	if err != nil {
		return
	}

	mutex.Lock()
	otherSources[elementName] = src
	mutex.Unlock()

	pipeline.SetState(gst.StatePlaying)

	return
}

func generateUniqueElementName() string {
	return fmt.Sprintf("%d", time.Now().UnixNano() % 100000)
}

func onEOS(s *Source) {
	log.Printf("EOS CALLBACK: %s", s.name)
}

func StopMain() {
	MainSource.pipeline.SendEvent(gst.NewEosEvent())
	<-MainSource.tearDownDone
	pipeline = nil
	MainSource = nil
	log.Print("Teardown done")
}