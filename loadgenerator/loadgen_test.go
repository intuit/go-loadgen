package loadgen

import (
	"fmt"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBuildLineMethod(t *testing.T) {
	var limit int = 10
	loadGen := new(LoadGenProperties)
	loadGen.LineLength = int64(limit)
	if len(loadGen.buildLine()) != limit {
		t.Error("buildLine() has generated a string which is not equal to the specified lineLength")
	}
}

func tearDownTestFiles(fileArray []*os.File) {
	for _, v := range fileArray {
		_ = os.Remove(v.Name())
	}
}

func TestMultiLineBuilder(t *testing.T) {
	loadGen := new(LoadGenProperties)
	var limit int = 10
	loadGen.LineLength = int64(limit)
	loadGen.NumOfLinesInMultiLineLog = 1
	expectedNumberOfLines := loadGen.NumOfLinesInMultiLineLog
	expectedNumberOfBytes := (int(loadGen.LineLength) * loadGen.NumOfLinesInMultiLineLog) + (loadGen.NumOfLinesInMultiLineLog - 1)
	fmt.Print(expectedNumberOfBytes)

	outputLine := loadGen.buildMultiLine()
	if len(outputLine) != expectedNumberOfBytes {
		t.Errorf("Multiline does not have the expected number of bytes. actual = %d, expected %d", len(outputLine), expectedNumberOfBytes)
	}
	lines := strings.Split(outputLine, "\n")
	if len(lines) != expectedNumberOfLines {
		t.Errorf("Multiline does not contain the expected number of new lines. actual = %d, expected %d", len(lines), expectedNumberOfLines)
	}

}

func TestFileRefs(t *testing.T) {
	var limit int = 10
	loadGen := new(LoadGenProperties)
	loadGen.FilePath = "xtest.log"
	loadGen.FileCount = int64(limit)
	files, err := loadGen.fileRef()
	defer tearDownTestFiles(files)

	if err != nil {
		t.Errorf("fileRef() method output is invalid %s", err)
	}
	if files == nil {
		t.Errorf("fileRef() output is nil, expected %d", limit)
	}
	if len(files) != limit {
		t.Errorf("length of file array returned by fileRef() has a mismatch, contains %d, expected %d", len(files), limit)
	}
}

func TestNumberedPath(t *testing.T) {
	for _, tc := range []struct{ path, suffix, want string }{
		{"test.log", "1", "test1.log"},
		{"/var/log/app.log", "-2", "/var/log/app-2.log"},
		{"./out.log", "0", "./out0.log"},
		{"/tmp/my.dir/test.log", "3", "/tmp/my.dir/test3.log"},
		{"/tmp/my.dir/test", "-1", "/tmp/my.dir/test-1"},
		{"test.tar.gz", "4", "test4.tar.gz"},
	} {
		if got := numberedPath(tc.path, tc.suffix); got != tc.want {
			t.Errorf("numberedPath(%q, %q) = %q, want %q", tc.path, tc.suffix, got, tc.want)
		}
	}
}

func TestOutputs(t *testing.T) {
	stdout := &LoadGenProperties{FilePath: "stdout", FileCount: 3}
	if outputs, err := stdout.outputs(); err != nil || len(outputs) != 1 || outputs[0] != os.Stdout {
		t.Errorf("stdout with --file-count 3: outputs = %v, err = %v; want only stdout", outputs, err)
	}

	// A "." in a directory name used to be replaced instead of the one in the file name.
	dir := filepath.Join(t.TempDir(), "my.dir")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := &LoadGenProperties{FilePath: filepath.Join(dir, "test.log"), FileCount: 2}
	outputs, err := files.outputs()
	if err != nil || len(outputs) != 2 {
		t.Fatalf("outputs = %v, err = %v; want 2 files", outputs, err)
	}
	for i, output := range outputs {
		if want := filepath.Join(dir, "test"+strconv.Itoa(i)+".log"); output.(*os.File).Name() != want {
			t.Errorf("output %d = %s, want %s", i, output.(*os.File).Name(), want)
		}
	}

	// Rotated files without an extension used to all share the same file.
	rotated := &LoadGenProperties{FilePath: filepath.Join(dir, "rotated"), FileCount: 2, Rotate: true}
	outputs, err = rotated.outputs()
	if err != nil || len(outputs) != 2 {
		t.Fatalf("outputs = %v, err = %v; want 2 rotated files", outputs, err)
	}
	if a, b := outputs[0].(*lumberjack.Logger).Filename, outputs[1].(*lumberjack.Logger).Filename; a == b {
		t.Errorf("both rotated outputs write to %s", a)
	}

	missingDir := &LoadGenProperties{FilePath: filepath.Join(dir, "missing", "test.log"), FileCount: 1}
	if _, err := missingDir.outputs(); err == nil {
		t.Error("expected an error for an output file in a directory that does not exist")
	}
}
