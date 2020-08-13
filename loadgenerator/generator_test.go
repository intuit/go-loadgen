package loadgen

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/intuit/go-loadgen/constants"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const fileRootPath string = constants.TestOutputFileRootPath

func setupProps(props *LoadGenProperties) {
	var wg sync.WaitGroup
	wg.Add(1)
	if props.LineLength == 0 {
		props.LineLength = 10
	}
	if props.Lps == 0 {
		props.Lps = 10
	}
	props.ResultLog = fileRootPath + "/" + "result.log"

	if props.Duration == 0 {
		props.Duration = 2
	}
	props.EnableMetrics = false

	props.FileCount = 1

	props.DisableTimestamp = true

	props.Wg = &wg

	if !props.Rotate {
		os.OpenFile(props.FilePath, os.O_CREATE, 0644)
	}
}

func TestGenerateRandomAlphaNumeric_singleLine(t *testing.T) {
	props := new(LoadGenProperties)
	props.MultiLinePercent = 0
	props.NumOfLinesInMultiLineLog = 1
	outputFilePath := fileRootPath + "/" + uuid.New().String()[:5] + ".log"
	props.FilePath = outputFilePath
	setupProps(props)
	GenerateAlphaNumeric(nil, props)
	totalNewLineCharacters := props.Duration * props.Lps
	expectedTotalBytesInFile := (props.Duration * props.Lps * props.LineLength) + totalNewLineCharacters
	expectedTotalBytesInFileWithErrorMargin := (props.Duration * props.Lps * props.LineLength) + (totalNewLineCharacters + props.LineLength + 1)

	defer os.Remove(outputFilePath)
	fmt.Println("Output path = " + outputFilePath)
	result, actualSize := hasExpectedLengthOfBytes(expectedTotalBytesInFile, outputFilePath)
	if !result && actualSize == expectedTotalBytesInFileWithErrorMargin {
		result = true
	}
	if !result {
		t.Errorf("The generated output file does not contain expected length of bytes. expected = %d, actual = %d", expectedTotalBytesInFile, actualSize)
	}
}

func TestGenerateRandomAlphaNumeric_multiLinePercent(t *testing.T) {
	props := new(LoadGenProperties)
	props.MultiLinePercent = 50
	props.NumOfLinesInMultiLineLog = 2
	outputFilePath := fileRootPath + "/" + uuid.New().String()[:5] + ".log"
	props.FilePath = outputFilePath
	props.Lps = 1
	props.Duration = 4
	//props.LogFormat = utility.GetFormatter(true, constants.DisableTimestamp)
	setupProps(props)
	GenerateAlphaNumeric(nil, props)
	totalNewLineCharacters := (props.Duration*props.Lps)/2 + (props.Duration*props.Lps*int64(props.NumOfLinesInMultiLineLog))/2
	expectedTotalBytesInFile := (props.Duration*props.Lps*props.LineLength)/2 +
		(props.Duration*props.Lps*props.LineLength*int64(props.NumOfLinesInMultiLineLog))/2 +
		totalNewLineCharacters

	defer os.Remove(outputFilePath)
	fmt.Println("Output path = " + outputFilePath)
	result, actualSize := hasExpectedLengthOfBytes(expectedTotalBytesInFile, outputFilePath)
	if !result {
		t.Errorf("The generated output file does not contain expected length of bytes. expected = %d, actual = %d", expectedTotalBytesInFile, actualSize)
	}
}

func deleteFile(files []string) {
	//cleanup any existing files before staring this test
	if len(files) > 0 {
		for _, v := range files {
			os.Remove(v)
		}
	}
}

func TestGenerateRandomAlphaNumeric_rotation(t *testing.T) {
	props := new(LoadGenProperties)
	props.MultiLinePercent = 0
	props.NumOfLinesInMultiLineLog = 1
	props.Rotate = true
	props.RotateSizeMB = 1     // 1MB
	props.LineLength = 1000000 //1MB
	props.Duration = 3         //in seconds
	props.Lps = 1              //in seconds
	props.FilePath = fileRootPath + "/" + uuid.New().String()[:5]

	setupProps(props)
	GenerateAlphaNumeric(nil, props)

	files, _ := filepath.Glob(props.FilePath + "*")
	defer deleteFile(files)
	expectedNumOfRotatedFiles := props.Duration * props.Lps

	if len(files) != int(expectedNumOfRotatedFiles) {
		t.Errorf("Log rotation validation tests failed because expected number of rotated files = %d, actual = %d", expectedNumOfRotatedFiles, len(files))
	}
}

func TestGenerateRandomAlphaNumericLoad_multiLine(t *testing.T) {
	props := new(LoadGenProperties)
	props.DisableTimestamp = true
	props.MultiLinePercent = 100 //100% of lines are multi-lines in nature.
	props.NumOfLinesInMultiLineLog = 2
	outputFilePath := uuid.New().String()[:5] + ".log"
	props.FilePath = outputFilePath
	setupProps(props)
	fmt.Printf("%+v", props)
	GenerateAlphaNumeric(nil, props)

	totalNewLineCharacters := props.Duration * props.Lps * int64(props.NumOfLinesInMultiLineLog)
	expectedTotalBytesInFile := props.Duration*props.Lps*int64(props.NumOfLinesInMultiLineLog)*props.LineLength + totalNewLineCharacters

	defer os.Remove(outputFilePath)
	fmt.Println("Output path = " + outputFilePath)
	result, actualSize := hasExpectedLengthOfBytes(expectedTotalBytesInFile, outputFilePath)
	if !result {
		t.Errorf("The generated output file does not contain expected length of bytes. expected = %d, actual = %d", expectedTotalBytesInFile, actualSize)
	}
}

func hasExpectedLengthOfBytes(expectedTotalBytesInFile int64, output string) (bool, int64) {

	info, err := os.Stat(output)
	if err != nil {
		return false, info.Size()
	}
	if approximatelyEqual(expectedTotalBytesInFile, info.Size()) {
		return true, info.Size()
	}
	return false, info.Size()
}

func approximatelyEqual(expected, actual int64) bool {
	errorMargin := 0.05 // 5% error is tolerable
	fivePercentOfExpected := float64(expected) * errorMargin
	diff := math.Abs(float64(expected - actual))
	fmt.Println(fivePercentOfExpected)
	fmt.Println(diff)
	if diff <= fivePercentOfExpected {
		return true
	}
	return false
}
