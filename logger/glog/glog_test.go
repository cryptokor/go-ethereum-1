// Go support for leveled logs, analogous to https://code.google.com/p/google-glog/
//
// Copyright 2013 Google Inc. All Rights Reserved.
// Modifications copyright 2017 ETC Dev Team. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package glog

import (
	"bytes"
	"fmt"
	stdLog "log"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Test that shortHostname works as advertised.
func TestShortHostname(t *testing.T) {
	for hostname, expect := range map[string]string{
		"":                "",
		"host":            "host",
		"host.google.com": "host",
	} {
		if got := shortHostname(hostname); expect != got {
			t.Errorf("shortHostname(%q): expected %q, got %q", hostname, expect, got)
		}
	}
}

// flushBuffer wraps a bytes.Buffer to satisfy flushSyncWriter.
type flushBuffer struct {
	bytes.Buffer
}

func (f *flushBuffer) Flush() error {
	return nil
}

func (f *flushBuffer) Sync() error {
	return nil
}

// swap sets the log writers and returns the old array.
func (l *loggingT) swap(writers [numSeverity]flushSyncWriter) (old [numSeverity]flushSyncWriter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	old = l.file
	for i, w := range writers {
		logging.file[i] = w
	}
	return
}

// newBuffers sets the log writers to all new byte buffers and returns the old array.
func (l *loggingT) newBuffers() [numSeverity]flushSyncWriter {
	return l.swap([numSeverity]flushSyncWriter{new(flushBuffer), new(flushBuffer), new(flushBuffer), new(flushBuffer)})
}

// contents returns the specified log value as a string.
func contents(s severity) string {
	return logging.file[s].(*flushBuffer).String()
}

// contains reports whether the string is contained in the log.
func contains(s severity, str string, t *testing.T) bool {
	return strings.Contains(contents(s), str)
}

// setFlags configures the logging flags how the test expects them.
func setFlags() {
	logging.toStderr = false
}

// Test that Info works as advertised.
func TestInfo(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	Info("test")
	if !contains(infoLog, "I", t) {
		t.Errorf("Info has wrong character: %q", contents(infoLog))
	}
	if !contains(infoLog, "test", t) {
		t.Error("Info failed")
	}
}

func TestInfoDepth(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())

	f := func() { InfoDepth(1, "depth-test1") }

	// The next three lines must stay together
	_, _, wantLine, _ := runtime.Caller(0)
	InfoDepth(0, "depth-test0")
	f()

	msgs := strings.Split(strings.TrimSuffix(contents(infoLog), "\n"), "\n")
	if len(msgs) != 2 {
		t.Fatalf("Got %d lines, expected 2", len(msgs))
	}

	for i, m := range msgs {
		if !strings.HasPrefix(m, severityColor[infoLog]+"I") {
			t.Errorf("InfoDepth[%d] has wrong character: %q", i, m)
		}
		w := fmt.Sprintf("depth-test%d", i)
		if !strings.Contains(m, w) {
			t.Errorf("InfoDepth[%d] missing %q: %q", i, w, m)
		}

		// pull out the line number (between : and ])
		msg := m[strings.LastIndex(m, ":")+1:]
		x := strings.Index(msg, "]")
		if x < 0 {
			t.Errorf("InfoDepth[%d]: missing ']': %q", i, m)
			continue
		}
		line, err := strconv.Atoi(msg[:x])
		if err != nil {
			t.Errorf("InfoDepth[%d]: bad line number: %q", i, m)
			continue
		}
		wantLine++
		if wantLine != line {
			t.Errorf("InfoDepth[%d]: got line %d, want %d", i, line, wantLine)
		}
	}
}

func init() {
	CopyStandardLogTo("INFO")
}

// Test that CopyStandardLogTo panics on bad input.
func TestCopyStandardLogToPanic(t *testing.T) {
	defer func() {
		if s, ok := recover().(string); !ok || !strings.Contains(s, "LOG") {
			t.Errorf(`CopyStandardLogTo("LOG") should have panicked: %v`, s)
		}
	}()
	CopyStandardLogTo("LOG")
}

// Test that using the standard log package logs to INFO.
func TestStandardLog(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	stdLog.Print("test")
	if !contains(infoLog, "I", t) {
		t.Errorf("Info has wrong character: %q", contents(infoLog))
	}
	if !contains(infoLog, "test", t) {
		t.Error("Info failed")
	}
}

// Test that the header has the correct format.
func TestHeader1ErrorLog(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	defer func(previous func() time.Time) { timeNow = previous }(timeNow)
	timeNow = func() time.Time {
		return time.Date(2006, 1, 2, 15, 4, 5, .067890e9, time.Local)
	}
	pid = 1234
	Error("test")
	var line int
	format := severityColor[errorLog] + "E" + severityColorReset + "0102 15:04:05.067890 logger/glog/glog_test.go:%d] test\n"
	n, err := fmt.Sscanf(contents(errorLog), format, &line)
	if n != 1 || err != nil {
		t.Errorf("log format error: %d elements, error %s:\n%s", n, err, contents(errorLog))
	}
	// Scanf treats multiple spaces as equivalent to a single space,
	// so check for correct space-padding also.
	want := fmt.Sprintf(format, line)
	if contents(errorLog) != want {
		t.Errorf("log format error: got:\n\t%q\nwant:\t%q", contents(errorLog), want)
	}
}

// Test that the header has the correct format.
func TestHeader2InfoLog(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	defer func(previous func() time.Time) { timeNow = previous }(timeNow)
	timeNow = func() time.Time {
		return time.Date(2006, 1, 2, 15, 4, 5, .067890e9, time.Local)
	}
	s := logging.verbosityTraceThreshold.get()
	logging.verbosityTraceThreshold.set(5) // Use app flag defaults
	defer logging.verbosityTraceThreshold.set(s)
	pid = 1234
	Info("test")
	format := severityColor[infoLog] + "I" + severityColorReset + "0102 15:04:05.067890 logger/glog/glog_test.go:209] test\n"
	n, err := fmt.Sscanf(contents(infoLog), format)
	if err != nil {
		t.Errorf("log format error: %d elements, error %s:\n%s", n, err, contents(infoLog))
	}
	// Scanf treats multiple spaces as equivalent to a single space,
	// so check for correct space-padding also.
	want := fmt.Sprintf(format)
	if contents(infoLog) != want {
		t.Errorf("log format error: got:\n\t%q\nwant:\t%q", contents(infoLog), want)
	}
}

// Test that an Error log goes to Warning and Info.
// Even in the Info log, the source character will be E, so the data should
// all be identical.
func TestError(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	Error("test")
	if !contains(errorLog, "E", t) {
		t.Errorf("Error has wrong character: %q", contents(errorLog))
	}
	if !contains(errorLog, "test", t) {
		t.Error("Error failed")
	}
	str := contents(errorLog)
	if !contains(warningLog, str, t) {
		t.Error("Warning failed")
	}
	if !contains(infoLog, str, t) {
		t.Error("Info failed")
	}
}

// Test that a Warning log goes to Info.
// Even in the Info log, the source character will be W, so the data should
// all be identical.
func TestWarning(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	Warning("test")
	if !contains(warningLog, "W", t) {
		t.Errorf("Warning has wrong character: %q", contents(warningLog))
	}
	if !contains(warningLog, "test", t) {
		t.Error("Warning failed")
	}
	str := contents(warningLog)
	if !contains(infoLog, str, t) {
		t.Error("Info failed")
	}
}

// Test that a V log goes to Info.
func TestV(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	logging.verbosity.Set("2")
	defer logging.verbosity.Set("0")
	V(2).Info("test")
	if !contains(infoLog, "I", t) {
		t.Errorf("Info has wrong character: %q", contents(infoLog))
	}
	if !contains(infoLog, "test", t) {
		t.Error("Info failed")
	}
}

// Test that a vmodule enables a log in this file.
func TestVmoduleOn(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	logging.vmodule.Set("glog_test.go=2")
	defer logging.vmodule.Set("")
	if !V(1) {
		t.Error("V not enabled for 1")
	}
	if !V(2) {
		t.Error("V not enabled for 2")
	}
	if V(3) {
		t.Error("V enabled for 3")
	}
	V(2).Info("test")
	if !contains(infoLog, "I", t) {
		t.Errorf("Info has wrong character: %q", contents(infoLog))
	}
	if !contains(infoLog, "test", t) {
		t.Error("Info failed")
	}
}

// Test that a vmodule of another file does not enable a log in this file.
func TestVmoduleOff(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	logging.vmodule.Set("notthisfile=2")
	defer logging.vmodule.Set("")
	for i := 1; i <= 3; i++ {
		if V(Level(i)) {
			t.Errorf("V enabled for %d", i)
		}
	}
	V(2).Info("test")
	if contents(infoLog) != "" {
		t.Error("V logged incorrectly")
	}
}

var patternTests = []struct{ input, want string }{
	{"foo/bar/x.go", ".*/foo/bar/x\\.go$"},
	{"foo/*/x.go", ".*/foo(/.*)?/x\\.go$"},
	{"foo/*", ".*/foo(/.*)?/[^/]+\\.go$"},
}

func TestCompileModulePattern(t *testing.T) {
	for _, test := range patternTests {
		re, err := compileModulePattern(test.input)
		if err != nil {
			t.Fatalf("%s: %v", test.input, err)
		}
		if re.String() != test.want {
			t.Errorf("mismatch for %q: got %q, want %q", test.input, re.String(), test.want)
		}
	}
}

// vGlobs are patterns that match/don't match this file at V=2.
var vGlobs = map[string]bool{
	// Easy to test the numeric match here.
	"glog_test.go=1": false, // If -vmodule sets V to 1, V(2) will fail.
	"glog_test.go=2": true,
	"glog_test.go=3": true, // If -vmodule sets V to 1, V(3) will succeed.

	// Import path prefix matching
	"logger/glog=1": false,
	"logger/glog=2": true,
	"logger/glog=3": true,

	// Import path glob matching
	"logger/*=1": false,
	"logger/*=2": true,
	"logger/*=3": true,

	// These all use 2 and check the patterns.
	"*=2": true,
}

// Test that vmodule globbing works as advertised.
func testVmoduleGlob(pat string, match bool, t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	defer logging.vmodule.Set("")
	logging.vmodule.Set(pat)
	if V(2) != Verbose(match) {
		t.Errorf("incorrect match for %q: got %t expected %t", pat, V(2), match)
	}
}

// Test that a vmodule globbing works as advertised.
func TestVmoduleGlob(t *testing.T) {
	for glob, match := range vGlobs {
		testVmoduleGlob(glob, match, t)
	}
}

func TestRollover(t *testing.T) {
	setFlags()
	var err error
	defer func(previous func(error)) { logExitFunc = previous }(logExitFunc)
	logExitFunc = func(e error) {
		err = e
	}
	defer func(previous uint64) { MaxSize = previous }(MaxSize)
	MaxSize = 512

	Info("x") // Be sure we have a file.
	info, ok := logging.file[infoLog].(*syncBuffer)
	if !ok {
		t.Fatal("info wasn't created")
	}
	if err != nil {
		t.Fatalf("info has initial error: %v", err)
	}
	fname0 := info.file.Name()
	Info(strings.Repeat("x", int(MaxSize))) // force a rollover
	if err != nil {
		t.Fatalf("info has error after big write: %v", err)
	}

	// Make sure the next log file gets a file name with a different
	// time stamp.
	//
	// TODO: determine whether we need to support subsecond log
	// rotation.  C++ does not appear to handle this case (nor does it
	// handle Daylight Savings Time properly).
	time.Sleep(1 * time.Second)

	Info("x") // create a new file
	if err != nil {
		t.Fatalf("error after rotation: %v", err)
	}
	fname1 := info.file.Name()
	if fname0 == fname1 {
		t.Errorf("info.f.Name did not change: %v", fname0)
	}
	if info.nbytes >= MaxSize {
		t.Errorf("file size was not reset: %d", info.nbytes)
	}
}

func TestLogBacktraceAt(t *testing.T) {
	setFlags()
	defer logging.swap(logging.newBuffers())
	// The peculiar style of this code simplifies line counting and maintenance of the
	// tracing block below.
	var infoLine string
	setTraceLocation := func(file string, line int, ok bool, delta int) {
		if !ok {
			t.Fatal("could not get file:line")
		}
		_, file = filepath.Split(file)
		infoLine = fmt.Sprintf("%s:%d", file, line+delta)
		err := logging.traceLocation.Set(infoLine)
		if err != nil {
			t.Fatal("error setting log_backtrace_at: ", err)
		}
	}
	{
		// Start of tracing block. These lines know about each other's relative position.
		_, file, line, ok := runtime.Caller(0)
		setTraceLocation(file, line, ok, +2) // Two lines between Caller and Info calls.
		Info("we want a stack trace here")
	}
	numAppearances := strings.Count(contents(infoLog), infoLine)
	if numAppearances < 2 {
		// Need 2 appearances, one in the log header and one in the trace:
		//   log_test.go:281: I0511 16:36:06.952398 02238 log_test.go:280] we want a stack trace here
		//   ...
		//   github.com/glog/glog_test.go:280 (0x41ba91)
		//   ...
		// We could be more precise but that would require knowing the details
		// of the traceback format, which may not be dependable.
		t.Fatal("got no trace back; log is ", contents(infoLog))
	}
}

func TestExtractTimestamp(t *testing.T) {
	preffix := fmt.Sprintf("%s.%s.%s.log.", "geth_test", "sampleHost", "sampleUser")
	cases := []struct {
		name     string
		fileName string
		expected string
	}{
		{"valid INFO", preffix + "INFO.20171202-132113.2841", "20171202-132113"},
		{"valid WARNIG", preffix + "WARNING.20171202-210922.13848", "20171202-210922"},
		{"valid gzipped", preffix + "WARNING.20171202-210922.13848.gz", "20171202-210922"},
		{"extra long filename", preffix + "WARNING.20171202-210922.13848.gz.bak", "20171202-210922"},
		{"to short filename", preffix + "WARNING.20171202-21092", ""},
		{"no preffix", "WARNING.20171202-21092", ""},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			actual := extractTimestamp(test.fileName, preffix)
			if test.expected != actual {
				t.Errorf("Expected: '%s', actual: '%s'", test.expected, actual)
			}
		})
	}
}

func TestShouldRotate(t *testing.T) {
	// fixed date, to make tests stable, 04.12.2017 is Monday
	start := time.Date(2017, time.December, 4, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		nbytes   uint64
		len      int
		now      time.Time
		minSize  uint64
		maxSize  uint64
		interval Interval
		expected bool
	}{
		{"empty log no rotation", 0, 123, start, 0, 0, Never, false},
		{"empty log with hourly rotation", 0, 123, start, 0, 0, Hourly, false},
		{"empty log with size rotation", 0, 123, start, 0, 1024 * 1024, Never, false},
		{"log with hourly rotation after less than hour", 1234, 123, start.Add(45 * time.Minute), 0, 0, Hourly, false},
		{"log with hourly rotation after more than hour", 1234, 123, start.Add(65 * time.Minute), 0, 0, Hourly, true},
		{"log with size rotation below MinSize", 1024, 123, start, 512 * 1024, 1024 * 1024, Never, false},
		{"log with size rotation between MinSize and MaxSize", 765 * 1024, 123, start, 512 * 1024, 1024 * 1024, Never, false},
		{"log with size rotation above MaxSize", 1024*1024 - 100, 123, start, 512 * 1024, 1024 * 1024, Never, true},
		{"log with daily rotation after less than day", 1234, 123, start.Add(23 * time.Hour), 0, 0, Daily, false},
		{"log with daily rotation after more than day", 1234, 123, start.Add(25 * time.Hour), 0, 0, Daily, true},
		{"log with weekly rotation after less than week", 1234, 123, start.Add((6*24 + 23) * time.Hour), 0, 0, Weekly, false},
		{"log with weekly rotation after more than week", 1234, 123, start.Add((7*24 + 1) * time.Hour), 0, 0, Weekly, true},
		{"log with monthly rotation after less than month", 1234, 123, start.Add(14 * 24 * time.Hour), 0, 0, Monthly, false},
		{"log with monthly rotation after more than month", 1234, 123, start.Add(30 * 24 * time.Hour), 0, 0, Monthly, true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sb := &syncBuffer{nbytes: test.nbytes, time: start}
			MinSize = test.minSize
			MaxSize = test.maxSize
			RotationInterval = test.interval
			actual := sb.shouldRotate(test.len, test.now)
			if test.expected != actual {
				t.Errorf("Expected: '%v', actual: '%v'", test.expected, actual)
			}
		})
	}
}

func BenchmarkHeader(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf, _, _ := logging.header(infoLog, 0)
		logging.putBuffer(buf)
	}
}
