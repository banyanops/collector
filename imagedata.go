// imagedata.go has funcitons that deal with the contents of images, including Linux distribution
// identification and application package names, versions, and architectures.
package collector

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
)

// ImageDataInfo describes a package included in the contents of an image.
type ImageDataInfo struct {
	Image        string //this has to be the first field (used in order by)
	DistroName   string //e.g., ubuntu 14.04.02 Trusty....
	DistroID     string //e.g., Trusty
	Pkg          string
	Version      string
	Architecture string
}

// PullImage performs a docker pull on an image specified by repo/tag.
// TODO: Detect if the pulled image has a different imageID than the value retrieved from
// metadata, and if so correct the metadata, or at least skip processing the image.
func PullImage(metadata ImageMetadataInfo) {
	tagspec := RegistrySpec + "/" + metadata.Repo + ":" + metadata.Tag
	apipath := "/images/create?fromImage=" + tagspec
	blog.Info("PullImages downloading %s, Image ID: %s", apipath, metadata.Image)
	config.BanyanUpdate("Pull", apipath, metadata.Image)
	resp, err := doDockerAPI(DockerTransport, "POST", apipath, []byte{}, XRegistryAuth)
	if err != nil {
		blog.Error(err, "PullImages failed for", RegistrySpec, metadata.Repo, metadata.Tag, metadata.Image)
	}
	if strings.Contains(string(resp), `"error":`) {
		blog.Error("PullImages error for %s/%s/%s", RegistrySpec, metadata.Repo, metadata.Tag)
	}
	blog.Trace(string(resp))
	return
}

// RemoveImages removes least recently pulled docker images from the local docker host.
func RemoveImages(PulledImages []ImageMetadataInfo, imageToMDMap map[string][]ImageMetadataInfo) {
	numRemoved := 0
	for _, imageMD := range PulledImages {
		// Get all metadata (repo/tags) associated with that image
		for _, metadata := range imageToMDMap[imageMD.Image] {
			// basespec := RegistrySpec + "/" + string(t.Repo) + ":"
			if ExcludeRepo[RepoType(metadata.Repo)] {
				continue
			}
			blog.Debug("Removing the following registry/repo:tag: " + RegistrySpec + "/" +
				metadata.Repo + ":" + metadata.Tag)
			apipath := "/images/" + RegistrySpec + "/" + metadata.Repo + ":" + metadata.Tag
			blog.Info("RemoveImages %s", apipath)
			config.BanyanUpdate("Remove", apipath)
			_, err := doDockerAPI(DockerTransport, "DELETE", apipath, []byte{}, "")
			if err != nil {
				blog.Error(err, "RemoveImages Repo:Tag", metadata.Repo, metadata.Tag,
					"image", metadata.Image)
			}
			numRemoved++
		}
	}

	blog.Info("Number of repo/tags removed this time around: %d", numRemoved)
	return
}

type HTTPStatusCodeError struct {
	error
	StatusCode int
}

func (s *HTTPStatusCodeError) Error() string {
	return "HTTP Status Code " + strconv.Itoa(s.StatusCode)
}

// RegistryQuery performs an HTTP GET operation from the registry and returns the response.
func RegistryQuery(client *http.Client, URL string) (response []byte, e error) {
	_, _, BasicAuth, XRegistryAuth = GetRegistryURL()
	req, e := http.NewRequest("GET", URL, nil)
	if e != nil {
		return nil, e
	}
	if BasicAuth != "" {
		req.Header.Set("Authorization", "Basic "+BasicAuth)
	}
	r, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	return
}

// GetImageAllData extracts content info from each pulled image. Currently it gets system package info.
func GetImageAllData(pulledImages ImageSet) (outMapMap map[string]map[string]interface{}) {
	//Map ImageID -> Script Map; Script Map: Script name -> output
	outMapMap = make(map[string]map[string]interface{})
	for imageID := range pulledImages {
		config.BanyanUpdate("Scripts", string(imageID))
		outMap, err := runAllScripts(imageID)
		if err != nil {
			blog.Error(err, ": Error processing image", string(imageID))
			continue
		}
		outMapMap[string(imageID)] = outMap
	}

	return
}

func statusMessageImageData(outMapMap map[string]map[string]interface{}) string {
	statString := ""
	for imageID, _ := range outMapMap {
		statString += imageID + ", "
		if len(statString) > maxStatusLen {
			return statString[0:maxStatusLen]
		}
	}
	return statString
}

// SaveImageAllData saves output of all the scripts.
func SaveImageAllData(outMapMap map[string]map[string]interface{} /*, dotfiles []DotFilesType*/) {
	config.BanyanUpdate("Save Image Data", statusMessageImageData(outMapMap))
	for _, writer := range WriterList {
		writer.WriteImageAllData(outMapMap)
	}

	return
}
