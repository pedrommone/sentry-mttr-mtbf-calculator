package main

import (
	"os"

	"github.com/pedrommone/sentry-mttr-mtbf-calculator/log"
	"github.com/Sirupsen/logrus"
	_ "github.com/joho/godotenv/autoload"
)

type Calculator struct {
	Log 	*logrus.Logger
}

var sentryURL = "https://sentry.io/api/"

func main() {
	calculator := NewCalculator()
	calculator.Start()
}

func NewCalculator() *Calculator {
	calc := new(Calculator)

	SentryToken := os.Getenv("SENTRY_TOKEN")

	if len(SentryToken) == 0 {
		panic("Sentry token need.")
	}

	calc.Log = log.NewLogrus()

	return calc;
}

func (c *Calculator) Start() {
	c.Log.Info("===========");
}
