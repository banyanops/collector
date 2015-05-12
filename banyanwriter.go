package main

import (
	"encoding/json"
	"time"

	blog "github.com/ccpaging/log4go"
)

// BanyanWriter is a Writer that uploads data to the Banyan web service.
type BanyanWriter struct {
	authToken string
}

func newBanyanWriter(authToken string) Writer {
	return BanyanWriter{
		authToken: authToken,
	}
}

// WriteImageAllData writes image (pkg and other) data using Banyan API
func (bw BanyanWriter) WriteImageAllData(outMapMap map[string]map[string]interface{}) {
	blog.Info("Writing image (pkg and other) data using Banyan API...")

	imageData := []ImageDataInfo{}
	for image, scriptMap := range outMapMap {
		for _, output := range scriptMap {
			switch output.(type) {
			case []ImageDataInfo:
				imageData = append(imageData, output.([]ImageDataInfo)...)
			case []byte:
				writeMiscData(image, bw.authToken, output.([]byte))
			default:
				blog.Warn("Not supporting this output type for Image All Data!")
			}
		}
	}

	if len(imageData) == 0 {
		blog.Warn("No image data to send to Banyan")
		return
	}

	blog.Info("Writing %d imagedata elmts", len(imageData))
	URL := *banyanURL + "/insert_image_data"
	jsonifyAndSendToBanyan(&imageData, "SaveImageData", bw.authToken, URL)
}

// AppendImageMetadata appends image metadata using Banyan API
func (bw BanyanWriter) AppendImageMetadata(imageMetadata []ImageMetadataInfo) {
	blog.Info("Appending image metadata using Banyan API...")
	if len(imageMetadata) == 0 {
		blog.Warn("No image metadata (append) to send to Banyan")
		return
	}
	blog.Info("Writing %d image metadata elmts", len(imageMetadata))
	URL := *banyanURL + "/insert_image_metadata"
	jsonifyAndSendToBanyan(&imageMetadata, "SaveImageMetadata", bw.authToken, URL)
}

// RemoveImageMetadata removes image metadata using Banyan API
func (bw BanyanWriter) RemoveImageMetadata(imageMetadata []ImageMetadataInfo) {
	blog.Info("Removing image metadata using Banyan API...")
	if len(imageMetadata) == 0 {
		blog.Warn("No image metadata (remove) to send to Banyan")
		return
	}
	blog.Info("Deleting %d image metadata elmts", len(imageMetadata))
	URL := *banyanURL + "/delete_image_metadata"
	jsonifyAndSendToBanyan(&imageMetadata, "RemoveObsoleteImageMetadata", bw.authToken, URL)
}

func writeMiscData(image string, authToken string, data []byte) {
	blog.Info("Writing miscellaneous data using Banyan API...")
	if data == nil {
		blog.Warn("No misc data to send to Banyan...")
		return
	}

	imageMiscData := struct {
		Image string
		Data  []byte
	}{string(image), data}
	// TODO: Change this to a more generic URL we support
	URL := *banyanURL + "/insert_pkg_dependencies"
	jsonifyAndSendToBanyan(&imageMiscData, "SaveMiscData", authToken, URL)
}

func jsonifyAndSendToBanyan(data interface{}, identifier string, authToken string, URL string) (err error) {
	b, err := json.Marshal(data)
	if err != nil {
		blog.Error(err, ": Failed to jsonify - "+identifier+" - while sending to Banyan")
		return
	}

	for {
		err := doPostBanyanAPI(identifier, authToken, URL, b)
		if err != nil {
			blog.Error(err, identifier+": retrying sending to Banyan API")
			time.Sleep(RETRYDURATION)
			continue
		}
		break
	}

	return
}
