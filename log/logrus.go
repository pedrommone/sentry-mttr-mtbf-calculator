package log

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/x-cray/logrus-prefixed-formatter"
)

// NewLogrus returns a new instance of Logrus
func NewLogrus() *logrus.Logger {
	log := logrus.New()
	log.Level = getLogLevel()
	log.Formatter = new(prefixed.TextFormatter)

	return log
}

func getLogLevel() logrus.Level {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	logl, err := logrus.ParseLevel(level)
	if err != nil {
		panic(err)
	}

	return logl
}
