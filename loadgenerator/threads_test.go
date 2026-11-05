package loadgen

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/intuit/go-loadgen/eventbreaker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// sampleLog has single-line events, a multi-line event (a stack trace) and no trailing newline.
const sampleLog = "2019-12-05 18:44:57 INFO request started\n" +
	"2019-12-05 18:44:58 ERROR request failed\n" +
	"java.lang.IllegalStateException: boom\n" +
	"\tat com.intuit.Foo.bar(Foo.java:42)\n" +
	"\tat com.intuit.Foo.main(Foo.java:7)\n" +
	"209.124.125.232 - franecki2473 [22/Dec/2022:18:56:54 +0000] \"GET /cultivate HTTP/2.0\" 403 5187\n" +
	"2019-12-05 18:44:59 INFO request finished"

var sampleEvents = []event{
	{"2019-12-05 18:44:57 INFO request started\n", 1},
	{"2019-12-05 18:44:58 ERROR request failed\njava.lang.IllegalStateException: boom\n" +
		"\tat com.intuit.Foo.bar(Foo.java:42)\n\tat com.intuit.Foo.main(Foo.java:7)\n", 4},
	{"209.124.125.232 - franecki2473 [22/Dec/2022:18:56:54 +0000] \"GET /cultivate HTTP/2.0\" 403 5187\n", 1},
	{"2019-12-05 18:44:59 INFO request finished\n", 1},
}

const sampleLines = 7

func parseEvents(t *testing.T, input string) []event {
	t.Helper()
	reader := bufio.NewReader(strings.NewReader(input))
	breakers := eventbreaker.NewEventBreakers()
	var events []event
	for {
		ev, ok, err := nextEvent(reader, breakers)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return events
		}
		events = append(events, ev)
	}
}

func writeInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.log")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readOutputs returns the concatenated content of the files matching pattern.
func readOutputs(t *testing.T, pattern string) string {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		t.Fatalf("no output files match %s: %v", pattern, err)
	}
	var content strings.Builder
	for _, file := range files {
		bytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content.Write(bytes)
	}
	return content.String()
}

func TestNextEvent(t *testing.T) {
	if got := parseEvents(t, sampleLog); !reflect.DeepEqual(got, sampleEvents) {
		t.Errorf("events =\n%#v\nwant\n%#v", got, sampleEvents)
	}
}

func TestNextEvent_multiLineLimit(t *testing.T) {
	input := "2019-12-05 18:44:57 start\n" + strings.Repeat("continued\n", 1499)
	events := parseEvents(t, input)
	if len(events) != 2 || events[0].lines != multiLineLimit || events[1].lines != 500 {
		t.Fatalf("expected events of %d and 500 lines, got %d events", multiLineLimit, len(events))
	}
}

// The last line of a file used to be treated as the start of a multi-line event, which read
// up to 1000 empty lines at the end of every replay (issue #4).
func TestReplay_singleLineFile(t *testing.T) {
	line := "209.124.125.232 - franecki2473 [22/Dec/2022:18:56:54 +0000] \"GET /cultivate HTTP/2.0\" 403 5187\n"
	props := &LoadGenProperties{
		InputSourceFile:  writeInput(t, line),
		FilePath:         filepath.Join(t.TempDir(), "out.log"),
		FileCount:        1,
		Lps:              1000,
		ReplayCount:      5,
		DisableTimestamp: true,
	}
	total, err := replayInputFile(nil, props)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total_lines_generated = %d, want 5", total)
	}
	if got, want := readOutputs(t, props.FilePath), strings.Repeat(line, 5); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
}

func TestReplay_threads(t *testing.T) {
	const replays = 50
	dir := t.TempDir()
	props := &LoadGenProperties{
		InputSourceFile:  writeInput(t, sampleLog),
		FilePath:         filepath.Join(dir, "out.log"),
		FileCount:        3,
		Threads:          4,
		Lps:              100000,
		ReplayCount:      replays,
		DisableTimestamp: true,
	}
	total, err := replayInputFile(nil, props)
	if err != nil {
		t.Fatal(err)
	}
	if total != replays*sampleLines {
		t.Errorf("total_lines_generated = %d, want %d", total, replays*sampleLines)
	}

	// Every event is written exactly --replay-count times, and the lines of a multi-line
	// event are never interleaved with lines written by other threads.
	counts := map[event]int{}
	for _, ev := range parseEvents(t, readOutputs(t, filepath.Join(dir, "out*.log"))) {
		counts[ev]++
	}
	if len(counts) != len(sampleEvents) {
		t.Errorf("found %d distinct events in the output, want %d", len(counts), len(sampleEvents))
	}
	for _, ev := range sampleEvents {
		if counts[ev] != replays {
			t.Errorf("event %q written %d times, want %d", ev.text, counts[ev], replays)
		}
	}
}

// Input files too large to be kept in memory are read again on every replay.
func TestReplay_threadsLargeFile(t *testing.T) {
	defer func(limit int64) { replayCacheLimit = limit }(replayCacheLimit)
	replayCacheLimit = 0

	const replays = 20
	dir := t.TempDir()
	props := &LoadGenProperties{
		InputSourceFile:  writeInput(t, sampleLog),
		FilePath:         filepath.Join(dir, "out.log"),
		FileCount:        2,
		Threads:          4,
		Lps:              100000,
		ReplayCount:      replays,
		DisableTimestamp: true,
	}
	total, err := replayInputFile(nil, props)
	if err != nil {
		t.Fatal(err)
	}
	if total != replays*sampleLines {
		t.Errorf("total_lines_generated = %d, want %d", total, replays*sampleLines)
	}
	counts := map[event]int{}
	for _, ev := range parseEvents(t, readOutputs(t, filepath.Join(dir, "out*.log"))) {
		counts[ev]++
	}
	for _, ev := range sampleEvents {
		if counts[ev] != replays {
			t.Errorf("event %q written %d times, want %d", ev.text, counts[ev], replays)
		}
	}
}

func TestFileEvents(t *testing.T) {
	f, err := os.Open(writeInput(t, sampleLog))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	source := &fileEvents{file: f, reader: bufio.NewReader(f), breakers: eventbreaker.NewEventBreakers(), replayCount: 2}
	var got []event
	for ev, ok := source.next(); ok; ev, ok = source.next() {
		got = append(got, ev)
	}
	if want := append(append([]event{}, sampleEvents...), sampleEvents...); !reflect.DeepEqual(got, want) {
		t.Errorf("events =\n%#v\nwant\n%#v", got, want)
	}
	if _, ok := source.next(); ok || source.err() != nil {
		t.Errorf("expected no more events and no error, got err = %v", source.err())
	}
}

func TestReplay_rotationAddsNoBlankLines(t *testing.T) {
	props := &LoadGenProperties{
		InputSourceFile:  writeInput(t, "2019-12-05 18:44:57 a\n2019-12-05 18:44:58 b\n"),
		FilePath:         filepath.Join(t.TempDir(), "out.log"),
		FileCount:        1,
		Lps:              1000,
		ReplayCount:      3,
		Rotate:           true,
		DisableTimestamp: true,
	}
	if _, err := replayInputFile(nil, props); err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("2019-12-05 18:44:57 a\n2019-12-05 18:44:58 b\n", 3)
	if got := readOutputs(t, props.FilePath); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
}

// --lines-per-second is the combined rate of all threads.
func TestReplay_threadsShareTheRate(t *testing.T) {
	props := &LoadGenProperties{
		InputSourceFile:  writeInput(t, sampleLog),
		FilePath:         filepath.Join(t.TempDir(), "out.log"),
		FileCount:        2,
		Threads:          8,
		Lps:              500,
		Duration:         2,
		DisableTimestamp: true,
	}
	if _, err := replayInputFile(nil, props); err != nil {
		t.Fatal(err)
	}
	events := int64(len(parseEvents(t, readOutputs(t, filepath.Join(filepath.Dir(props.FilePath), "out*.log")))))
	if expected := props.Lps * props.Duration; !approximatelyEqual(expected, events) {
		t.Errorf("wrote %d events in %ds at %d events/s, want about %d", events, props.Duration, props.Lps, expected)
	}
}

func TestGenerateAlphaNumeric_threads(t *testing.T) {
	dir := t.TempDir()
	props := &LoadGenProperties{
		FilePath:                 filepath.Join(dir, "out.log"),
		FileCount:                4,
		Threads:                  8,
		Lps:                      1000,
		Duration:                 2,
		LineLength:               50,
		NumOfLinesInMultiLineLog: 1,
		DisableTimestamp:         true,
	}
	total, err := generateAlphaNumeric(nil, props)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(readOutputs(t, filepath.Join(dir, "out*.log")), "\n"), "\n")
	if int64(len(lines)) != total {
		t.Errorf("found %d lines in the output files, total_lines_generated = %d", len(lines), total)
	}
	if expected := props.Lps * props.Duration; !approximatelyEqual(expected, total) {
		t.Errorf("wrote %d lines in %ds at %d lines/s, want about %d", total, props.Duration, props.Lps, expected)
	}
	for _, line := range lines {
		if len(line) != int(props.LineLength) {
			t.Fatalf("found a line of %d characters, want %d: %q", len(line), props.LineLength, line)
		}
	}
}

// Two tests in the same process used to panic with "duplicate metrics collector
// registration attempted".
func TestConcurrentTestsShareMetricsRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	dir := t.TempDir()
	var wg sync.WaitGroup
	for _, name := range []string{"a.log", "b.log"} {
		props := &LoadGenProperties{
			FilePath:                 filepath.Join(dir, name),
			FileCount:                1,
			Threads:                  2,
			Lps:                      100,
			Duration:                 1,
			LineLength:               10,
			NumOfLinesInMultiLineLog: 1,
			EnableMetrics:            true,
			DisableTimestamp:         true,
		}
		wg.Go(func() {
			if total, err := generateAlphaNumeric(registry, props); err != nil || total == 0 {
				t.Errorf("%s: total_lines_generated = %d, err = %v", name, total, err)
			}
		})
	}
	wg.Wait()
}

func TestExportMetrics(t *testing.T) {
	props := &LoadGenProperties{FilePath: filepath.Join(t.TempDir(), "out.log"), FileCount: 1, EnableMetrics: true, DisableTimestamp: true}
	g, err := newGeneration(props, true)
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	unregister := g.exportMetrics(registry)
	g.write(0, 0, "hello", 1)
	g.write(0, 0, "world!", 1)

	expected := `
# HELP total_bytes_processed Total bytes processed
# TYPE total_bytes_processed counter
total_bytes_processed 11
# HELP total_events_processed Total log events generated by the tool.
# TYPE total_events_processed counter
total_events_processed 2
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected)); err != nil {
		t.Error(err)
	}
	unregister()
	if count, err := testutil.GatherAndCount(registry); err != nil || count != 0 {
		t.Errorf("%d metrics left in the registry after unregister, err = %v", count, err)
	}
}
