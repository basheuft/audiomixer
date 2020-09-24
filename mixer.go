package audiomixer

import (
	"fmt"
	"github.com/notedit/gst"
	"time"
)

var audioCaps = gst.CapsFromString("audio/x-raw,format=S16LE,layout=interleaved,rate=44100,channels=2")

func GenerateUniqueElementName() string {
	return fmt.Sprintf("%d", time.Now().UnixNano() % 100000)
}