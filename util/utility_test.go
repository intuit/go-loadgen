package utility

import (
	"github.com/google/uuid"
	"github.com/intuit/go-loadgen/constants"
	"github.com/intuit/go-loadgen/eventbreaker"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestGetFormatter(t *testing.T) {
	outputFile := constants.TestOutputFileRootPath + "/" + uuid.New().String()[:5] + ".log"
	file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_RDWR, 0666)
	defer os.Remove(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	log := logrus.New()
	log.Out = file
	log.SetFormatter(GetFormatter(false))
	testString := "This is a test!"
	log.Info(testString)
	bytes, err := ioutil.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	fileContents := string(bytes)
	eb := eventbreaker.NewEventBreakers()
	result := eb.HasKnownEventBreakerString(fileContents)
	if !result {
		t.Fatal("Well known date pattern not found. Formatter has an error")
	}

	if !strings.Contains(fileContents, testString) {
		t.Fatal("Test string not found in the file after formatting!")
	}
}
