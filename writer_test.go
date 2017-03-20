package collector

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	fsutil "github.com/banyanops/collector/fsutil"
	blog "github.com/ccpaging/log4go"
)

// TestWriteImageAllData tests writing different types of image data to files
func TestWriteImageAllData(t *testing.T) {
	cases := []struct {
		script, image, destDir, format string
	}{
		{"myscript", "image", "/tmp", "json"},
		{"myscript.sh", "image1234", "/tmp", "json"},
		{"myscript.abc.sh", "aaaabbbb", "/tmp", "json"},
	}
	outMap := make(map[string]interface{})
	outMapMap := make(map[string]map[string]interface{})

	// Testing imagedata...
	var idata = []ImageDataInfo{{"111", "a", "b", "c", "dn1", "did1"}, {"111", "d", "e", "f", "dn2", "did2"}, {"121", "g", "h", "i", "dn3", "did3"}}
	for _, c := range cases {
		outMap[c.script] = idata
		outMapMap[c.image] = outMap
		b1 := testWriteToFile(t, outMapMap, c.script, c.image, c.destDir, c.format, "-pkgdata")
		b2, err := json.MarshalIndent(idata, "", "\t")
		if err != nil {
			t.Fatal(err, ": Error in marshaling json for imagedata")
		}
		if !bytes.Equal(b1, b2) {
			blog.Debug(b1)
			blog.Debug(b2)
			t.Fatal("Input/Output image data don't match: ", len(b1), len(b2))
		}
	}

	blog.Info("Reaching here => writing imagedata to file works fine")

	// Testing random output ([]byte)...
	randOut := []byte("Testing random output from scripts")
	for _, c := range cases {
		script := "X" + c.script
		outMap[script] = randOut
		outMapMap[c.image] = outMap
		b := testWriteToFile(t, outMapMap, script, c.image, c.destDir, "txt", "-miscdata")
		if !bytes.Equal(b, randOut) {
			blog.Debug(b)
			blog.Debug(randOut)
			t.Fatal("Input/Output image rand txt don't match", len(b), len(randOut))
		}
	}

	//Pass...
	return
}

func testWriteToFile(t *testing.T, outMapMap map[string]map[string]interface{}, script, image, destDir, format string, suffix string) (b []byte) {
	fw := NewFileWriter(format, destDir)
	fw.WriteImageAllData(outMapMap)
	// Test if correct output file exists
	finalDir := destDir + "/" + trimExtension(script) + "/"
	blog.Debug("final dir: " + finalDir)
	var filenamePath string
	if ok, e := fsutil.DirExists(finalDir); ok {
		if len(image) > 12 {
			image = image[0:12]
		}
		file := image + suffix + "." + format
		filenamePath = finalDir + "/" + file
		_, err := os.Stat(filenamePath)
		if err != nil {
			if os.IsNotExist(err) {
				t.Fatal(err, ": File ", filenamePath, " doesn't exist")
			}
			t.Fatal(err, ": Unknown error while locating file: ", filenamePath)
		}
	} else {
		t.Fatal(e, ": Directory: ", finalDir, " doesn't exist")
	}

	b, err := ioutil.ReadFile(filenamePath)
	if err != nil {
		t.Fatal(err, ": Error in reading file: ", filenamePath)
	}
	return b
}

// TestWriteImageMetadata tests writing (appending/removing) imageMD to file
func TestWriteImageMetadata(t *testing.T) {
	const (
		format  = "json"
		destDir = "/tmp"
	)

	// Testing imagedata...
	var imdata = []ImageMetadataInfo{
		{"111", time.Now(), OtherMetadata{"r1", "t1", 100, "a1", "c1", "c1", "p1"}, "", ""},
		{"121", time.Now(), OtherMetadata{"r2", "t2", 100, "a2", "c2", "c2", "p2"}, "", ""},
		{"131", time.Now(), OtherMetadata{"r3", "t3", 100, "a3", "c3", "c3", "p3"}, "", ""},
	}

	// Remove output file if it already exists -- since we append
	file := "metadata." + format
	filenamePath := destDir + "/" + file
	if _, err := os.Stat(filenamePath); err == nil {
		// file exists
		e := os.Remove(filenamePath)
		if e != nil {
			t.Fatal(": Error in removing metadata file: ", filenamePath)
		}
	} //ignore else

	// Append to MD file
	b1 := testWriteImageMDToFile(t, imdata, "/tmp", "json", "ADD")
	data1 := ImageMetadataAndAction{"ADD", imdata}
	b2, err := json.MarshalIndent(data1, "", "\t")
	if err != nil {
		t.Fatal(err, ": Error in marshaling json")
	}
	if !bytes.Equal(b1, b2) {
		t.Fatal("Input/Output image metadata don't match")
	}

	// "Remove" from MD file (note that action is set to remove, rather than really removing anything)
	b3 := testWriteImageMDToFile(t, []ImageMetadataInfo{imdata[0]}, "/tmp", "json", "REMOVE")
	data2 := ImageMetadataAndAction{"REMOVE", []ImageMetadataInfo{imdata[0]}}
	b4, err := json.MarshalIndent(data2, "", "\t")
	b5 := append(b2, b4...)
	if err != nil {
		t.Fatal(err, ": Error in marshaling json")
	}
	if !bytes.Equal(b3, b5) {
		blog.Info(string(b5))
		t.Fatal("Input/Output image metadata don't match: ", len(b3), len(b5))
	}
	//Pass...
	return
}

func testWriteImageMDToFile(t *testing.T, imageMD []ImageMetadataInfo, destDir, format, action string) (b []byte) {
	// Append/Remove
	fw := NewFileWriter(format, destDir)
	switch action {
	case "ADD":
		fw.AppendImageMetadata(imageMD)
	case "REMOVE":
		fw.RemoveImageMetadata(imageMD)
	}

	// Check vailidity of output files
	var filenamePath string
	if ok, e := fsutil.DirExists(destDir); ok {
		file := "metadata." + format
		filenamePath = destDir + "/" + file
		_, err := os.Stat(filenamePath)
		if err != nil {
			if os.IsNotExist(err) {
				t.Fatal(err, ": File ", filenamePath, " doesn't exist")
			}
			t.Fatal(err, ": Unknown error while locating file: ", filenamePath)
		}
	} else {
		t.Fatal(e, ": Directory: ", destDir, " doesn't exist")
	}

	// Read output and return
	b, err := ioutil.ReadFile(filenamePath)
	if err != nil {
		t.Fatal(err, ": Error in reading file: ", filenamePath)
	}

	return b
}
