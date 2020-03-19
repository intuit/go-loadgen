package loadgen

import (
	"os"
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

func tearDownTestFiles(fileArray []*os.File){
	for _,v := range fileArray {
		_ = os.Remove(v.Name())
	}
}

func TestMultiLineBuilder(t *testing.T) {
	loadGen := new(LoadGenProperties)
	var limit int = 10
	loadGen.LineLength = int64(limit)
	loadGen.NumOfLinesInMultiLineLog = 10
	expectedNumberOfLines := loadGen.NumOfLinesInMultiLineLog
	expectedNumberOfBytes := (int(loadGen.LineLength) * loadGen.NumOfLinesInMultiLineLog) + (loadGen.NumOfLinesInMultiLineLog - 1)

	outputLine  := loadGen.buildMultiLine()
	if len(outputLine) != expectedNumberOfBytes {
		t.Errorf("Multiline does not have the expected number of bytes. actual = %d, expected %d", len(outputLine), expectedNumberOfBytes)
	}
	lines := strings.Split(outputLine, "\n")
	if len(lines) != expectedNumberOfLines {
		t.Errorf("Multiline does not contain the expected number of new lines. actual = %d, expected %d", len(lines),  expectedNumberOfLines)
	}

}

func TestFileRefs(t *testing.T) {
	var limit int = 10
	loadGen := new(LoadGenProperties)
	loadGen.FilePath = "xtest.log"
	loadGen.FileCount = int64(limit)
	files, err := loadGen.fileRef()
	defer tearDownTestFiles(files)

	if err != nil  {
		t.Errorf("fileRef() method output is invalid %s",err)
	}
	if files == nil {
		t.Errorf("fileRef() output is nil, expected %d",limit)
	}
	if len(files) != limit {
		t.Errorf("length of file array returned by fileRef() has a mismatch, contains %d, expected %d",len(files), limit)
	}
}


