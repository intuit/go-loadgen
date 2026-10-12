package loadgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/intuit/go-loadgen/constants"
	utility "github.com/intuit/go-loadgen/util"
	"github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type LoadGenProperties struct {
	Duration                 int64           `json:"duration"`
	FileCount                int64           `json:"file-count"`
	Threads                  int             `json:"threads"`
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

	rawJsonConfig, err := io.ReadAll(config)
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

// threads returns the number of generator goroutines. The zero value means one, which
// is how the generator has always behaved.
func (props *LoadGenProperties) threads() int {
	if props.Threads < 1 {
		return 1
	}
	return props.Threads
}

// isStdout reports whether the output is stdout rather than a file path.
func (props *LoadGenProperties) isStdout() bool {
	return strings.EqualFold(props.FilePath, "stdout")
}

// numberedPath inserts suffix into the file name of path, in front of the first "." of
// the file name, e.g. ("/var/log/app.log", "1") -> "/var/log/app1.log". Dots in the
// directory part of the path are left alone.
func numberedPath(path, suffix string) string {
	dir, file := filepath.Split(path)
	if i := strings.Index(file, "."); i >= 0 {
		return dir + file[:i] + suffix + file[i:]
	}
	return dir + file + suffix
}

/**
 * Creates n number of files and returns an array of pointer references to the created files.
 */
func (props *LoadGenProperties) fileRef() ([]*os.File, error) {
	// if output path is declared as stdout then return reference to stdout and skip rest
	if props.isStdout() {
		return []*os.File{os.Stdout}, nil
	}
	var fileArray = make([]*os.File, 0, props.FileCount)
	for n := 0; n < int(props.FileCount); n++ {
		fileVar := props.FilePath
		if props.FileCount > 1 {
			//attach file number before the file extension
			fileVar = numberedPath(fileVar, strconv.Itoa(n))
		}
		file, err := os.OpenFile(fileVar, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return fileArray, err
		}
		fileArray = append(fileArray, file)
	}
	return fileArray, nil
}

// rotatingFiles returns a size-rotated log file per --file-count.
func (props *LoadGenProperties) rotatingFiles() []io.Writer {
	maxFileSize := int(props.RotateSizeMB)
	if maxFileSize == 0 {
		maxFileSize = constants.DefaultMaxFileRotationSize
	}
	files := make([]io.Writer, props.FileCount)
	for i := range files {
		fileName := props.FilePath
		if props.FileCount > 1 {
			fileName = numberedPath(fileName, "-"+strconv.Itoa(i))
		}
		files[i] = &lumberjack.Logger{
			Filename:   fileName,
			MaxSize:    maxFileSize, //megabytes
			MaxAge:     1,           //day
			MaxBackups: 3,           //max of n files.
			Compress:   false,       //dont compress to avoid CPU spikes on load generator.
		}
	}
	return files
}

// outputs opens the destinations a test writes to: stdout, or --file-count files with or
// without log rotation. Lines are spread round-robin over the outputs.
func (props *LoadGenProperties) outputs() ([]io.Writer, error) {
	if props.FileCount < 1 && !props.isStdout() {
		return nil, errors.New("at least one output file is required")
	}
	if props.Rotate && !props.isStdout() {
		return props.rotatingFiles(), nil
	}
	files, err := props.fileRef()
	if err != nil {
		return nil, err
	}
	outputs := make([]io.Writer, len(files))
	for i, file := range files {
		outputs[i] = file
	}
	return outputs, nil
}

// loggers returns a logger per output. The loggers are shared by all generator threads:
// logrus writes each entry with a single Write call under the logger's lock, so lines
// written by different threads to the same output never interleave.
func (props *LoadGenProperties) loggers(outputs []io.Writer, isLineEndsWithNewLine bool) []*logrus.Logger {
	var formatter logrus.Formatter
	if props.LogFormat != nil {
		formatter = props.LogFormat
	} else {
		formatter = utility.GetFormatter(SetupLogProps(false, isLineEndsWithNewLine, props))
	}
	loggers := make([]*logrus.Logger, len(outputs))
	for i, output := range outputs {
		loggers[i] = logrus.New()
		loggers[i].SetOutput(output)
		loggers[i].SetFormatter(formatter)
	}
	return loggers
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

func SetupLogProps(isMetricsLogs bool, insertNewline bool, props *LoadGenProperties) *utility.LogProperties {
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
		if insertNewline {
			logProps.IsLineEndsWithNewLine = true
		} else {
			logProps.IsLineEndsWithNewLine = false
		}
	}

	return logProps
}
