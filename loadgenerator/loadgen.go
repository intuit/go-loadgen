package loadgen

import (
	"encoding/json"
	"fmt"
	"github.com/intuit/go-loadgen/constants"
	utility "github.com/intuit/go-loadgen/util"
	"github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"gopkg.in/natefinch/lumberjack.v2"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type LoadGenProperties struct {
	Duration                 int64           `json:"duration"`
	FileCount                int64           `json:"file-count"`
	LineLength               int64           `json:"line-length"`
	MultiLinePercent         int             `json:"multiline-percentage"`
	NumOfLinesInMultiLineLog int             `json:"line-count"`
	FilePath                 string          `json:"file-path"`
	InputSourceFile          string          `json:"input-source-file"`
	ReplayCount              int64           `json:"replay-count"`
	CustomTimestampFormat    string          `json:"custom-timestamp-format"`
	DisableTimestamp         bool            `json:"disable-timestamp"`
	Lps                      int64           `json:"lines-per-second"`
	Rotate                   bool            `json:"rotate"`
	RotateSizeMB             int64           `json:"rotation-max-file-size-megabytes"`
	EnableMetrics            bool            `json:"enable-metrics"`
	Tags                     string          `json:"tags"`
	LogFormat                *easy.Formatter `json:"-"`
	Wg                       *sync.WaitGroup `json:"-"`
	ResultLog                string          `json:"results-log"`
}

const chars = "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ" + "1234567890"

func (props *LoadGenProperties) readFromConfig(configFile *string) {
	config, err := os.Open(*configFile)
	defer config.Close()
	if err != nil && *configFile != "" {
		fmt.Println(err)
		os.Exit(1)
	}

	rawJsonConfig, err := ioutil.ReadAll(config)
	if err != nil && *configFile != "" {
		fmt.Println(err)
		os.Exit(1)
	}
	err = json.Unmarshal(rawJsonConfig, &props)
	if err != nil && *configFile != "" {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (props *LoadGenProperties) durationInSeconds() time.Duration {
	return time.Second * (time.Duration(props.Duration))
}

func (props *LoadGenProperties) fetchLogHandlers(isLineEndsWithNewLine bool) []*logrus.Logger {
	logHandlers := make([]*logrus.Logger, props.FileCount)
	for n := 0; n < int(props.FileCount); n++ {
		logHandlers[n] = logrus.New()
		if props.LogFormat == nil {
			logProps := SetupLogProps(false, props)
			logHandlers[n].SetFormatter(utility.GetFormatter(logProps))
		} else {
			logHandlers[n].SetFormatter(props.LogFormat)
		}
	}
	return logHandlers
}

/**
 * Creates n number of files and returns an array of pointer references to the created files.
 */
func (props *LoadGenProperties) fileRef() ([]*os.File, error) {
	var fileArray = make([]*os.File, props.FileCount, props.FileCount)

	// if output path is declared as stdout then return reference to stdout and skip rest
	if props.FilePath != "" {
		if strings.ToLower(props.FilePath) == "stdout" {
			fileArray[0] = os.Stdout
			return fileArray, nil
		}
	}
	var err error = nil
	for n := 0; n < int(props.FileCount); n++ {
		fileVar := props.FilePath
		if props.FileCount > 1 {
			if strings.Contains(fileVar, ".") {
				//attach file number before the file extension
				fileVar = strings.Replace(fileVar, ".", strconv.Itoa(n)+".", 1)
			} else {
				fileVar = fileVar + strconv.Itoa(n)
			}
		}
		fileArray[n], err = os.OpenFile(fileVar, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			break
		}
	}
	return fileArray, err
}

func (props *LoadGenProperties) buildLine() string {
	var seededRand = rand.New(
		rand.NewSource(time.Now().UnixNano()))
	randString := make([]byte, props.LineLength)
	for i, _ := range randString {
		randNumber := seededRand.Intn(int(len(chars) - 1))
		randString[i] = chars[randNumber]
	}

	return string(randString)
}

func (props *LoadGenProperties) setupLogRotation(filePath string, logHandlers []*logrus.Logger) {

	for i := 0; i < len(logHandlers); i++ {
		var maxFileSize int = 0
		if props.RotateSizeMB == 0 {
			maxFileSize = constants.DefaultMaxFileRotationSize
		} else {
			maxFileSize = int(props.RotateSizeMB)
		}

		var suffix string
		if props.FileCount > 1 {
			suffix = "-" + strconv.Itoa(i)
		}
		if props.FileCount == 1 {
			suffix = ""
		}

		(logHandlers)[i].SetOutput(&lumberjack.Logger{
			Filename:   strings.Replace(filePath, ".", suffix+".", 1),
			MaxSize:    maxFileSize, //megabytes
			MaxAge:     1,           //day
			MaxBackups: 3,           //max of n files.
			Compress:   false,       //dont compress to avoid CPU spikes on load generator.
		})
	}

}

func (props *LoadGenProperties) buildMultiLine() string {
	lineLength := int(props.LineLength)
	numberOfLines := int(props.NumOfLinesInMultiLineLog)
	maxCapacityRequired := (numberOfLines * (lineLength)) + (numberOfLines - 1)
	multiLineString := make([]byte, maxCapacityRequired, maxCapacityRequired)
	totalLineLength := 0
	for i := 0; i < numberOfLines; i++ {

		copy(multiLineString[totalLineLength:], []byte(props.buildLine()))
		totalLineLength = totalLineLength + lineLength
		if i < numberOfLines {
			copy(multiLineString[totalLineLength:], []byte("\n"))
		}
		totalLineLength += 1
	}
	return string(multiLineString)
}

func SetupLogProps(isMetricsLogs bool, props *LoadGenProperties) *utility.LogProperties {
	logProps := new(utility.LogProperties)

	if isMetricsLogs {
		logProps.DisableTimestamp = false
		logProps.CustomTimestampFormat = ""
		logProps.Tags = ""
		logProps.IsLineEndsWithNewLine = true
	} else {
		logProps.DisableTimestamp = props.DisableTimestamp
		logProps.CustomTimestampFormat = props.CustomTimestampFormat
		logProps.Tags = props.Tags
		logProps.IsLineEndsWithNewLine = true
	}

	return logProps
}
