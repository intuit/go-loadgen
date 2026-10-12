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
	"sync"
	"sync/atomic"
	"time"
)

// multiLineLimit is the maximum number of lines a single replayed log event can span.
const multiLineLimit = 1000

// replayCacheLimit is the size (in bytes) up to which an input file is parsed once and
// replayed from memory. Larger input files are read again on every replay.
var replayCacheLimit int64 = 32 << 20

func wrapUpTask(props *LoadGenProperties, totalLineCount int64) {
	var log = logrus.New()
	if props.ResultLog == "" {
		fmt.Printf("ResultLog file was not specified for this test. ResultLog file helps summarize the outcome of the test. " +
			"It is a recommended parameter.")
	} else {
		resultsLog, err := os.OpenFile(props.ResultLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
		log.Out = resultsLog
		log.Info(props.Tags+" total_lines_generated=", totalLineCount)
		resultsLog.Close()
	}

	fmt.Printf("\n\n total_lines_generated=%v\n\n ", totalLineCount)
	props.Wg.Done()
}

// generation is the state of a single test, shared by its generator threads.
type generation struct {
	props    *LoadGenProperties
	loggers  []*logrus.Logger // one per output
	deadline time.Time        // zero when the test does not stop after --duration
	stats    []threadStats    // one per thread
}

// threadStats counts what a generator thread wrote. Every thread updates its own counters,
// padded to 128 bytes so that they never share a cache line with the counters of another
// thread, and threads do not slow each other down.
type threadStats struct {
	events atomic.Int64 // log events written
	lines  atomic.Int64 // lines written, reported as total_lines_generated
	bytes  atomic.Int64 // bytes of log events written, excluding timestamps and tags
	_      [128 - 3*8]byte
}

// totals is what all the threads of a test have written so far.
type totals struct {
	events, lines, bytes int64
}

func newGeneration(props *LoadGenProperties, isLineEndsWithNewLine bool) (*generation, error) {
	outputs, err := props.outputs()
	if err != nil {
		return nil, err
	}
	g := &generation{props: props, loggers: props.loggers(outputs, isLineEndsWithNewLine)}
	g.stats = make([]threadStats, g.threads())
	return g, nil
}

// threads returns the number of generator threads: --threads, but no more than
// --lines-per-second since every thread writes at least one line per second.
func (g *generation) threads() int {
	threads := g.props.threads()
	if g.props.Lps > 0 && int64(threads) > g.props.Lps {
		return int(g.props.Lps)
	}
	return threads
}

// limiter returns the rate limiter of a generator thread. --lines-per-second is split
// evenly between the threads, and every thread has a limiter of its own so that threads
// never wait for each other.
func (g *generation) limiter(thread int) ratelimit.Limiter {
	if g.props.Lps <= 0 {
		return ratelimit.NewUnlimited()
	}
	threads := int64(len(g.stats))
	rate := g.props.Lps / threads
	if int64(thread) < g.props.Lps%threads {
		rate++
	}
	// Sleeps shorter than a millisecond can take a millisecond or more, depending on the
	// OS. Let the limiter catch up on up to 20ms of oversleeping, rather than on 10 lines
	// by default, or high rates are never reached.
	return ratelimit.New(int(rate), ratelimit.WithSlack(max(10, int(rate/50))))
}

// start starts the test clock. A timed test is over once --duration has elapsed.
func (g *generation) start(timed bool) {
	if timed {
		g.deadline = time.Now().Add(g.props.durationInSeconds())
	}
}

// expired reports whether a timed test has reached its deadline. Threads check it after
// every rate limiter permit, so no line is written after the deadline.
func (g *generation) expired() bool {
	return !g.deadline.IsZero() && !time.Now().Before(g.deadline)
}

// run runs thread on every generator goroutine, with the goroutine's index and rate
// limiter, and waits for all of them to return. Threads start writing at different
// outputs and then go round-robin over the outputs.
func (g *generation) run(thread func(id int, limiter ratelimit.Limiter)) {
	var wg sync.WaitGroup
	for id := range g.stats {
		wg.Go(func() {
			limiter := g.limiter(id)
			// Spread out the threads' writes evenly instead of all threads writing at once.
			if g.props.Lps > 0 {
				time.Sleep(time.Duration(id) * time.Second / time.Duration(g.props.Lps))
			}
			thread(id, limiter)
		})
	}
	wg.Wait()
}

// write writes a log event of n lines to the output with the given index.
func (g *generation) write(thread, output int, text string, lines int64) {
	g.loggers[output].Info(text)
	stats := &g.stats[thread]
	stats.events.Add(1)
	stats.lines.Add(lines)
	stats.bytes.Add(int64(len(text)))
}

func (g *generation) totals() totals {
	var t totals
	for i := range g.stats {
		t.events += g.stats[i].events.Load()
		t.lines += g.stats[i].lines.Load()
		t.bytes += g.stats[i].bytes.Load()
	}
	return t
}

// eventsCounter is the number of log events a test wrote, as a read-only go-metrics
// counter for the metrics logged to stdout every second.
type eventsCounter struct{ g *generation }

func (c eventsCounter) Count() int64              { return c.g.totals().events }
func (c eventsCounter) Snapshot() metrics.Counter { return metrics.CounterSnapshot(c.Count()) }
func (eventsCounter) Clear()                      {}
func (eventsCounter) Inc(int64)                   {}
func (eventsCounter) Dec(int64)                   {}

// exportMetrics exports the test metrics through the Prometheus registry when
// --enable-metrics is set, and returns a func that removes them from the registry.
func (g *generation) exportMetrics(promRegistry *prometheus.Registry) (unregister func()) {
	if !g.props.EnableMetrics || promRegistry == nil {
		return func() {}
	}
	var registered []prometheus.Collector
	for _, collector := range []prometheus.Collector{
		metricsUtility.NewEventsProcessedCounterFunc(func() float64 { return float64(g.totals().events) }),
		metricsUtility.NewTotalBytesProcessedCounterFunc(func() float64 { return float64(g.totals().bytes) }),
	} {
		if err := promRegistry.Register(collector); err != nil {
			fmt.Println("Unable to export prometheus metrics:", err)
			continue
		}
		registered = append(registered, collector)
	}
	return func() {
		for _, collector := range registered {
			promRegistry.Unregister(collector)
		}
	}
}

/*
*
GenerateLoadFromInputFile replays an input sample file from the top of the head and adheres to the specified format, line rate and duration.
*/
func GenerateLoadFromInputFile(promRegistry *prometheus.Registry, props *LoadGenProperties) {

	if props.ReplayCount == 0 && props.durationInSeconds() == 0 {
		fmt.Print("Error! Stopping the test. \n" +
			" Test termination factor is missing! Either specify `--duration` or `replay-count`")
		os.Exit(1)
	}

	fmt.Println("Generating logs by replaying input file...")
	totalLineCount, err := replayInputFile(promRegistry, props)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wrapUpTask(props, totalLineCount)
}

// replayInputFile runs a replay test and returns the number of lines written.
func replayInputFile(promRegistry *prometheus.Registry, props *LoadGenProperties) (int64, error) {
	f, err := os.Open(props.InputSourceFile)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	source, err := newEventSource(f, props.ReplayCount)
	if err != nil {
		return 0, err
	}

	if props.Rotate {
		fmt.Println("log rotation enabled")
	} else {
		fmt.Println("log rotation is disabled")
	}
	// Replayed events already end with the newline they were read with.
	g, err := newGeneration(props, false)
	if err != nil {
		return 0, err
	}

	goGenMetricsRegistry := metrics.NewRegistry()
	goGenMetricsRegistry.Register("total-events-processed", eventsCounter{g})
	metricsLogger := logrus.New()
	metricsLogger.Out = os.Stdout
	logProps := SetupLogProps(false, true, props)
	metricsLogger.Formatter = utility.GetFormatter(logProps)

	go metrics.Log(goGenMetricsRegistry, 1*time.Second, metricsLogger)
	metrics.NewRegisteredFunctionalGauge("total-bytes-processed", goGenMetricsRegistry, func() int64 {
		return g.totals().bytes
	})
	defer g.exportMetrics(promRegistry)()

	g.start(props.durationInSeconds() > 0)
	g.run(func(id int, limiter ratelimit.Limiter) {
		output := id % len(g.loggers)
		for {
			ev, ok := source.next()
			if !ok {
				return
			}
			limiter.Take()
			if g.expired() {
				return
			}
			g.write(id, output, ev.text, ev.lines)
			output = (output + 1) % len(g.loggers)
		}
	})
	return g.totals().lines, source.err()
}

// event is a log event read from the input file: a single line, or a line followed by
// its continuation lines (a stack trace for example).
type event struct {
	text  string
	lines int64
}

// eventSource hands out the events of the input file to the generator threads, in the
// order of the file, replaying the file --replay-count times (or until the test is over
// when --replay-count is 0).
type eventSource interface {
	// next returns the next event to write, or false when there are no more events.
	next() (event, bool)
	// err returns the error that stopped the replay early, if any.
	err() error
}

func newEventSource(f *os.File, replayCount int64) (eventSource, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	breakers := eventbreaker.NewEventBreakers()
	reader := bufio.NewReader(f)
	if !info.Mode().IsRegular() || info.Size() > replayCacheLimit {
		return &fileEvents{file: f, reader: reader, breakers: breakers, replayCount: replayCount}, nil
	}

	var events []event
	for {
		ev, ok, err := nextEvent(reader, breakers)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		events = append(events, ev)
	}
	return &memoryEvents{events: events, total: replayCount * int64(len(events))}, nil
}

// memoryEvents replays the events of an input file small enough to be kept in memory.
// Threads take turns through the events with an atomic cursor.
type memoryEvents struct {
	events []event
	total  int64 // events to write, 0 when the file is replayed until the test is over
	cursor atomic.Int64
}

func (s *memoryEvents) next() (event, bool) {
	i := s.cursor.Add(1) - 1
	if len(s.events) == 0 || (s.total > 0 && i >= s.total) {
		return event{}, false
	}
	return s.events[i%int64(len(s.events))], true
}

func (s *memoryEvents) err() error {
	return nil
}

// fileEvents replays a large input file by reading it again on every replay.
type fileEvents struct {
	mu          sync.Mutex
	file        *os.File
	reader      *bufio.Reader
	breakers    *eventbreaker.KnownEventBreakers
	replayCount int64
	replays     int64 // completed replays
	read        int64 // events read in the current replay
	done        bool
	readErr     error
}

func (s *fileEvents) next() (event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for !s.done {
		ev, ok, err := nextEvent(s.reader, s.breakers)
		if err != nil {
			s.readErr, s.done = err, true
			break
		}
		if ok {
			s.read++
			return ev, true
		}
		// End of the file: start the next replay, unless that was the last one or the file
		// has no events at all.
		s.replays++
		if s.read == 0 || (s.replayCount > 0 && s.replays >= s.replayCount) {
			s.done = true
			break
		}
		s.read = 0
		if _, err := s.file.Seek(0, io.SeekStart); err != nil {
			s.readErr, s.done = err, true
			break
		}
		s.reader.Reset(s.file)
	}
	return event{}, false
}

func (s *fileEvents) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readErr
}

// nextEvent reads the next log event from reader, and returns false at the end of the
// input. An event is a line plus every following line that does not start with a known
// event breaker (a timestamp or an IP address), such as the lines of a stack trace, up to
// multiLineLimit lines.
func nextEvent(reader *bufio.Reader, breakers *eventbreaker.KnownEventBreakers) (event, bool, error) {
	currentLine, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return event{}, false, fmt.Errorf("read file line error: %w", err)
	}
	if currentLine == "" {
		return event{}, false, nil
	}

	ev := event{text: currentLine, lines: 1}
	if continuesEvent(reader, breakers) {
		var multiLineString strings.Builder
		multiLineString.WriteString(currentLine)
		for ev.lines < multiLineLimit && continuesEvent(reader, breakers) {
			nextLine, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return event{}, false, fmt.Errorf("read file line error: %w", err)
			}
			multiLineString.WriteString(nextLine)
			ev.lines++
		}
		ev.text = multiLineString.String()
	}
	// The last line of a file may not end with a newline. Add one, otherwise it would run
	// into the first line of the next replay.
	if !strings.HasSuffix(ev.text, "\n") {
		ev.text += "\n"
	}
	return ev, true, nil
}

// continuesEvent reports whether the next line of the input belongs to the current event:
// there is a next line, and it does not start with a known event breaker.
func continuesEvent(reader *bufio.Reader, breakers *eventbreaker.KnownEventBreakers) bool {
	nextLine, _ := reader.Peek(40)
	return len(nextLine) > 0 && !breakers.HasKnownEventBreakerBytes(nextLine)
}

/*
*
GenerateAlphaNumeric generates random alphanumeric strings and writes it to a specified file at specified line rate(lps) and for specified duration.
*/
func GenerateAlphaNumeric(promRegistry *prometheus.Registry, props *LoadGenProperties) {
	fmt.Print("Generating random alphanumeric characters...")
	totalLineCount, err := generateAlphaNumeric(promRegistry, props)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wrapUpTask(props, totalLineCount)
}

// generateAlphaNumeric runs a random strings test and returns the number of lines written.
func generateAlphaNumeric(promRegistry *prometheus.Registry, props *LoadGenProperties) (int64, error) {
	if props.Rotate {
		fmt.Println("log rotation enabled")
	}
	g, err := newGeneration(props, true)
	if err != nil {
		return 0, err
	}

	singleLineResult := props.buildLine()
	multiLineResult := props.buildMultiLine()

	//metrics
	goGenMetricsRegistry := metrics.NewRegistry()
	goGenMetricsRegistry.Register("total_events_processed", eventsCounter{g})
	metricsLogger := logrus.New()
	metricsLogger.Out = os.Stdout

	go metrics.Log(goGenMetricsRegistry, 1*time.Second, metricsLogger)
	metrics.NewRegisteredFunctionalGauge("bytes-per-second", goGenMetricsRegistry, func() int64 {
		return props.LineLength * props.Lps
	})
	defer g.exportMetrics(promRegistry)()

	g.start(true)
	g.run(func(id int, limiter ratelimit.Limiter) {
		output := id % len(g.loggers)
		var totalLineCount, multiLineCount float64
		for {
			limiter.Take()
			if g.expired() {
				return
			}
			// Each thread keeps its own share of multi-line events at --multi-line-percent;
			// the first event of a thread is multi-line.
			line := singleLineResult
			if props.MultiLinePercent != 0 &&
				(totalLineCount == 0 || int(multiLineCount/totalLineCount*100) <= props.MultiLinePercent) {
				line = multiLineResult
				multiLineCount++
			}
			g.write(id, output, line, 1)
			output = (output + 1) % len(g.loggers)
			totalLineCount++
		}
	})
	return g.totals().lines, nil
}
