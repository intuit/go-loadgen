package loadgen

import (
	"bufio"
	"fmt"
	"github.com/intuit/go-loadgen/eventbreaker"
	metricsUtility "github.com/intuit/go-loadgen/metrics"
	utility "github.com/intuit/go-loadgen/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rcrowley/go-metrics"
	"github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"
	"io"
	"os"
	"strings"
	"time"
)

func wrapUpTask(props *LoadGenProperties, totalLineCount int64) {
	var log = logrus.New()
	resultsLog, err := os.OpenFile(props.ResultLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	log.Out = resultsLog
	log.Info(props.Tags+" total_lines_generated=", totalLineCount)
	fmt.Printf("total_lines_generated=%v\n", totalLineCount)
	props.Wg.Done()
}

/**
GenerateLoadFromInputFile replays an input sample file from the top of the head and adheres to the specified format, line rate and duration.
*/
func GenerateLoadFromInputFile(promRegistry *prometheus.Registry, props *LoadGenProperties) {
	fmt.Println("Generating logs by replaying input file...")
	const MultiLineLimit = 1000

	var log = logrus.New()
	var totalLineCount int64
	var fileArray = make([]*os.File, props.FileCount, props.FileCount)
	var logHandlers = make([]*logrus.Logger, props.FileCount)
	f, err := os.Open(props.InputSourceFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if props.Rotate == true {
		fmt.Println("log rotation enabled")
		logHandlers = props.fetchLogHandlers(false)
		props.setupLogRotation(props.FilePath, logHandlers)
	} else {
		fmt.Println("log rotation is disabled")
		fileArray, _ = props.fileRef()
	}

	rateLimit := ratelimit.New(int(props.Lps))
	timer := time.After(props.durationInSeconds())
	numberOfFiles := int(props.FileCount)
	var multiLineString strings.Builder

	eventBreakers := eventbreaker.NewEventBreakers()
	fileCountIndex := 0
	var totalBytesProcessed int64

	counter := metrics.NewCounter()
	goGenMetricsRegistry := metrics.NewRegistry()
	goGenMetricsRegistry.Register("total-events-processed", counter)
	metricsLogger := logrus.New()
	metricsLogger.Out = os.Stdout
	metricsLogger.Formatter = utility.GetFormatter(false)
	go metrics.Log(goGenMetricsRegistry, 1*time.Second, metricsLogger)

	//prometheus stuff
	var promCounter, promTotalBytesProcessedCounter prometheus.Counter
	if props.EnableMetrics {
		promCounter = metricsUtility.GetEventsProcessedCounter()
		promTotalBytesProcessedCounter = metricsUtility.GetTotalBytesProcessedCounter()
		promRegistry.MustRegister(promCounter)
		promRegistry.MustRegister(promTotalBytesProcessedCounter)
	}

	metrics.NewRegisteredFunctionalGauge("total-bytes-processed", goGenMetricsRegistry, func() int64 {
		if props.EnableMetrics {
			promTotalBytesProcessedCounter.Add(float64(totalBytesProcessed))
		}
		return totalBytesProcessed
	})

	var fileLoopCount int64
	for {
		if props.ReplayCount != 0 && props.ReplayCount == fileLoopCount {
			wrapUpTask(props, totalLineCount)
			return
		}
		fileLoopCount++

		f.Seek(0, io.SeekStart)
		reader := bufio.NewReader(f)
		for {
			now := rateLimit.Take()

			if props.ReplayCount > 0 {
				select {
				case _ = <-timer:
					wrapUpTask(props, totalLineCount)
					return
				default:
				}
			}
			counter.Inc(1)
			if props.EnableMetrics {
				promCounter.Add(1)
			}

			prev := time.Now()
			if !props.Rotate {
				log.Out = fileArray[fileCountIndex]
				log.SetFormatter(utility.GetFormatter(false))
			} else {
				log = logHandlers[fileCountIndex]
			}
			currentLine, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatalf("read file line error: %v", err)
				return
			}

			nextLine, err := reader.Peek(40)
			isNotMultiLine := eventBreakers.HasKnownEventBreakerString(currentLine) && eventBreakers.HasKnownEventBreakerBytes(nextLine)
			if isNotMultiLine {
				log.Info(currentLine)
				totalLineCount++
				totalBytesProcessed += int64(len(currentLine))
			} else {
				currentMultiLineCount := 0
				for {
					multiLineString.WriteString(currentLine)
					currentMultiLineCount++
					totalLineCount++
					nextLineBytes, _ := reader.Peek(40)
					if eventBreakers.HasKnownEventBreakerBytes(nextLineBytes) || currentMultiLineCount >= MultiLineLimit {
						break
					}
					currentLine, _ = reader.ReadString('\n')
				}
				log.Info(multiLineString.String())
				totalBytesProcessed += int64(multiLineString.Len())
				multiLineString.Reset()
			}

			if fileCountIndex == numberOfFiles-1 {
				fileCountIndex = 0
			} else {
				fileCountIndex++
			}
			now.Sub(prev)
			prev = now
		}
	}
	f.Close()
}

/**
GenerateAlphaNumeric generates random alphanumeric strings and writes it to a specified file at specified line rate(lps) and for specified duration.
*/
func GenerateAlphaNumeric(promRegistry *prometheus.Registry, props *LoadGenProperties) {
	fmt.Print("Generating random alphanumeric characters...")
	var log = logrus.New()
	var fileArray = make([]*os.File, props.FileCount, props.FileCount)
	var logHandlers = make([]*logrus.Logger, props.FileCount, props.FileCount)
	if props.Rotate == true {
		fmt.Println("log rotation enabled")
		logHandlers = props.fetchLogHandlers(true)
		props.setupLogRotation(props.FilePath, logHandlers)
	} else {
		fileArray, _ = props.fileRef()
	}

	//rate limiter settings
	rateLimit := ratelimit.New(int(props.Lps))
	//duration settings
	timer := time.After(props.durationInSeconds())

	var totalLineCount float64
	var singleLineCount float64
	var multiLineCount float64
	singleLineResult := props.buildLine()
	multiLineResult := props.buildMultiLine()
	numberOfFiles := int(props.FileCount)
	fileCountIndex := 0
	prev := time.Now()

	//metrics
	counter := metrics.NewCounter()
	goGenMetricsRegistry := metrics.NewRegistry()
	goGenMetricsRegistry.Register("total_events_processed", counter)
	metricsLogger := logrus.New()
	metricsLogger.Out = os.Stdout
	metricsLogger.Formatter = utility.GetFormatter(true)
	go metrics.Log(goGenMetricsRegistry, 1*time.Second, metricsLogger)

	//prometheus stuff
	var promCounter, promTotalBytesProcessedCounter prometheus.Counter
	if props.EnableMetrics {
		promCounter = metricsUtility.GetEventsProcessedCounter()
		promTotalBytesProcessedCounter = metricsUtility.GetTotalBytesProcessedCounter()
		promRegistry.MustRegister(promCounter)
		promRegistry.MustRegister(promTotalBytesProcessedCounter)
	}

	metrics.NewRegisteredFunctionalGauge("bytes-per-second", goGenMetricsRegistry, func() int64 {
		value := props.LineLength * props.Lps
		//export to prom
		if props.EnableMetrics {
			promTotalBytesProcessedCounter.Add(float64(value))
		}
		return value
	})

	for {
		now := rateLimit.Take()
		select {
		case _ = <-timer:
			wrapUpTask(props, int64(totalLineCount))
			return
		default:
			{
				counter.Inc(1)
				if props.EnableMetrics {
					promCounter.Add(1)
				}

			}
		}
		if !props.Rotate {
			log.Out = fileArray[fileCountIndex]
			if props.LogFormat == nil {
				log.SetFormatter(utility.GetFormatter(true))
			} else {
				log.SetFormatter(props.LogFormat)
			}
		} else {
			log = logHandlers[fileCountIndex]
		}

		if int(multiLineCount/totalLineCount*100) <= props.MultiLinePercent && props.MultiLinePercent != 0 {
			log.Info(multiLineResult)
			multiLineCount++
		} else {
			log.Info(singleLineResult)
			singleLineCount++
		}
		if fileCountIndex == numberOfFiles-1 {
			fileCountIndex = 0
		} else {
			fileCountIndex++
		}
		now.Sub(prev)
		prev = now
		totalLineCount++
	}
}
