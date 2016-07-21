package collector

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	except "github.com/banyanops/collector/except"
	fsutil "github.com/banyanops/collector/fsutil"
	blog "github.com/ccpaging/log4go"
)

type ImageMetadataAndAction struct {
	Action        string
	ImageMetadata []ImageMetadataInfo
}

type FileWriter struct {
	format string
	dir    string
}

func NewFileWriter(format string, dir string) Writer {
	if format == "" {
		format = "json"
	}
	// format can be overwritten, hence the &
	return &FileWriter{
		format: format,
		dir:    dir,
	}
}

// WriteImageAllData writes image (pkg and other) data into file
func (f *FileWriter) WriteImageAllData(outMapMap map[string]map[string]interface{}) {
	blog.Info("Writing image (pkg and other) data into file...")

	for imageID, scriptMap := range outMapMap {
		for scriptName, out := range scriptMap {
			scriptDir := f.dir + "/" + trimExtension(scriptName)
			err := fsutil.CreateDirIfNotExist(scriptDir)
			if err != nil {
				except.Error(err, ": Error creating script dir: ", scriptDir)
				continue
			}
			image := string(imageID)
			minLen := 12
			index := strings.Index(image, ":")
			if index >= 0 {
				minLen += index + 1
			}
			if len(image) < minLen {
				except.Warn("Weird...Haven't seen imageIDs so small -- possibly a test?")
			} else {
				image = string(imageID)[0:minLen]
			}
			filenamePath := scriptDir + "/" + image
			if _, ok := out.([]byte); ok {
				f.format = "txt"
				filenamePath += "-miscdata"
			} else {
				// by default it is json. But f.format could get overwritten at any point
				// in the for loop if the output type is []byte, hence the (re)assignment
				f.format = "json"
				// NOTE: If we start using json for output other than imageData, change this
				filenamePath += "-pkgdata"
			}
			f.writeFileInFormat(filenamePath, &out)
		}
	}
	return
}

// AppendImageMetadata appends image metadata to file
func (f *FileWriter) AppendImageMetadata(imageMetadata []ImageMetadataInfo) {
	blog.Info("Appending image metadata to file...")
	f.format = "json"
	f.handleImageMetadata(imageMetadata, "ADD")
}

// RemoveImageMetadata removes image metadata from file
func (f *FileWriter) RemoveImageMetadata(imageMetadata []ImageMetadataInfo) {
	blog.Info("Removing image metadata from file...")
	f.format = "json"
	f.handleImageMetadata(imageMetadata, "REMOVE")
}

func (f *FileWriter) handleImageMetadata(imageMetadata []ImageMetadataInfo, action string) {
	if len(imageMetadata) == 0 {
		except.Warn("No image metadata to append to file...")
		return
	}

	// If output directory does not exist, first create it
	fsutil.CreateDirIfNotExist(f.dir)
	filenamePath := f.dir + "/" + "metadata"

	data := ImageMetadataAndAction{action, imageMetadata}
	f.appendFileInFormat(filenamePath, data)
}

func jsonifyAndWriteToFile(filenamePath string, data interface{}) (err error) {
	b, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		except.Error(err, ": Error in marshaling json")
		return err
	}

	err = ioutil.WriteFile(filenamePath, b, 0644)
	if err != nil {
		except.Error(err, ": Error in writing to file: ", filenamePath)
		return err
	}

	return nil
}

func (f *FileWriter) writeFileInFormat(filenamePath string, data interface{}) {
	blog.Info("Writing " + filenamePath + "...")
	switch f.format {
	case "json":
		err := jsonifyAndWriteToFile(filenamePath+".json", data)
		if err != nil {
			except.Error(err, ": Error in writing json output into file: ", filenamePath+".json")
			return
		}
	case "txt":
		// what's passed in is ptr to interface{}. First get interface{} out of it and then
		// typecast that to []byte
		err := ioutil.WriteFile(filenamePath+".txt", (*(data.(*interface{}))).([]byte), 0644)
		if err != nil {
			except.Error(err, ": Error in writing to file: ", filenamePath)
			return
		}
	default:
		except.Warn("Currently only supporting json output to write to files")
	}
}

func trimExtension(nameExt string) (name string) {
	extension := filepath.Ext(nameExt)
	name = nameExt[0 : len(nameExt)-len(extension)]
	return
}

func jsonifyAndAppendToFile(filenamePath string, data ImageMetadataAndAction) (err error) {
	b, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		except.Error(err, ": Error in marshaling json")
		return err
	}

	fd, err := os.OpenFile(filenamePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		except.Error(err, ": Error in opening file: ", filenamePath)
		return err
	}
	defer fd.Close()

	_, err = fd.Write(b)
	if err != nil {
		except.Error(err, ": Error in writing to file: ", filenamePath)
		return err
	}

	return nil
}

func (f *FileWriter) appendFileInFormat(filenamePath string, data ImageMetadataAndAction) {
	switch f.format {
	case "json":
		err := jsonifyAndAppendToFile(filenamePath+".json", data)
		if err != nil {
			except.Error(err, ": Error in writing json output into file: ", filenamePath+".json")
			return
		}
	default:
		except.Warn("Currently only supporting json output to write to files")
	}
}
