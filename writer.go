package collector

import (
	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
	"strings"
)

// This is a writer plugin interface. Currently supported plugins are:
// "fileWriter":   writes to a file in desired format, and
// "banyanWriter": invokes banyan API to send data to SAAS dashboard
type Writer interface {
	// Write output obtained by all the scripts to the appropriate writer plugin
	// Note: outMapMap maps: ImageID -> Script -> Output
	WriteImageAllData(outMapMap map[string]map[string]interface{})

	// Append Image metadata to the appropriate writer plugin
	AppendImageMetadata(imageMetadata []ImageMetadataInfo)

	// Remoe Image metadta from the appropriate writer plugin
	RemoveImageMetadata(imageMetadata []ImageMetadataInfo)
}

var (
	WriterList []Writer
)

func SetOutputWriters(authToken string) {
	dests := strings.Split(*config.Dests, ",")
	for _, dest := range dests {
		var writer Writer
		switch dest {
		case "file":
			writer = newFileWriter("json", *config.BanyanOutDir)
		case "banyan":
			writer = newBanyanWriter(authToken)
		default:
			blog.Error("No such output writer!")
			//ignore the rest and keep going
			continue
		}
		WriterList = append(WriterList, writer)
	}
}
