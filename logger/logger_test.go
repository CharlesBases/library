package logger

import (
	"testing"
	"time"
)

func TestSeelog(t *testing.T) {
	UseSeelog(WithFilename("./log.log"))

	Trace("trace")
	Debug("debug")
	Info("info")
	Error("error")

	<-time.NewTicker(time.Second).C
}
