// imagedata.go has funcitons that deal with the contents of images, including Linux distribution
// identification and application package names, versions, and architectures.
package collector

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
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
func PullImage(metadata ImageMetadataInfo) (err error) {
	tagspec := RegistrySpec + "/" + metadata.Repo + ":" + metadata.Tag
	apipath := "/images/create?fromImage=" + tagspec
	blog.Info("PullImage downloading %s, Image ID: %s", apipath, metadata.Image)
	config.BanyanUpdate("Pull", apipath, metadata.Image)
	resp, err := DockerAPI(DockerTransport, "POST", apipath, []byte{}, XRegistryAuth)
	if err != nil {
		except.Error(err, "PullImage failed for", RegistrySpec, metadata.Repo, metadata.Tag, metadata.Image)
		return
	}
	if strings.Contains(string(resp), `"error":`) {
		except.Error("PullImage error for %s/%s/%s", RegistrySpec, metadata.Repo, metadata.Tag)
		err = errors.New("PullImage error for " + RegistrySpec + "/" + metadata.Repo + "/" + metadata.Tag)
	}
	blog.Trace(string(resp))

	if metadata.Image > "" {
		// verify the image ID of the pulled image matches the expected metadata.
		imageMap, err := GetLocalImages(false, false)
		if err != nil {
			except.Error(err, ": PullImage unable to list local images")
		}
	OUTERLOOP:
		for imageID, repotagSlice := range imageMap {
			for _, repotag := range repotagSlice {
				if string(repotag.Repo) == metadata.Repo && string(repotag.Tag) == metadata.Tag {
					if string(imageID) == metadata.Image {
						// image IDs match, we're all good.
						break OUTERLOOP
					}
					// image ID doesn't match. Remove the image and return an error.
					newMetadata := metadata
					newMetadata.Image = string(imageID)
					RemoveImages([]ImageMetadataInfo{newMetadata})
					err = errors.New("PullImage " + metadata.Repo + ":" + metadata.Tag +
						" image ID " + string(imageID) + " doesn't match metadata-derived ID " +
						metadata.Image)
					return err
				}
			}
		}
	}
	return
}

// RemoveImages removes least recently pulled docker images from the local docker host.
func RemoveImages(PulledImages []ImageMetadataInfo) {
	numRemoved := 0
	imageMap, err := GetLocalImages(false, false)
	if err != nil {
		except.Error(err, ": RemoveImages unable to list local images")
	}
	for _, metadata := range PulledImages {
		if strings.HasPrefix(metadata.Repo, "library/") {
			metadata.Repo = strings.Replace(metadata.Repo, "library/", "", 1)
		}
		imageID := ImageIDType(metadata.Image)
		if metadata.Image == "" {
			// unknown image ID. Search the repotags for a match
			var err error
			imageID, err = imageMap.Image(RepoType(metadata.Repo), TagType(metadata.Tag))
			if err != nil {
				except.Error(err, ": RemoveImages unable to find image ID")
				break
			}
		}

		// Get all repo:tags associated with the image
		repoTagSlice := imageMap.RepoTags(imageID)
		if len(repoTagSlice) == 0 {
			except.Error("RemoveImages unable to find expected repo:tag " + metadata.Repo +
				":" + metadata.Tag + " for image ID=" + string(imageID))
			except.Error("imageMap is %v", imageMap)
			continue
		}
		for _, repotag := range repoTagSlice {
			// basespec := RegistrySpec + "/" + string(t.Repo) + ":"
			if ExcludeRepo[RepoType(repotag.Repo)] {
				continue
			}
			apipath := "/images/" + string(repotag.Repo) + ":" + string(repotag.Tag)
			blog.Info("RemoveImages %s", apipath)
			config.BanyanUpdate("Remove", apipath)
			_, err := DockerAPI(DockerTransport, "DELETE", apipath, []byte{}, "")
			if err != nil {
				except.Error(err, "RemoveImages Repo:Tag", repotag.Repo, repotag.Tag,
					"image", metadata.Image)
			}
			numRemoved++
		}
	}

	blog.Info("Number of repo/tags removed this time around: %d", numRemoved)

	RemoveDanglingImages()
	return
}

// RemoveDanglingImages deletes any dangling images (untagged and unreferenced intermediate layers).
func RemoveDanglingImages() (e error) {
	dangling, err := ListDanglingImages()
	if err != nil {
		except.Error(err, "RemoveDanglingImages")
		return err
	}
	if len(dangling) == 0 {
		return
	}

	for _, image := range dangling {
		_, err = RemoveImageByID(image)
		if err != nil {
			except.Error(err, "RemoveDanglingImages")
			e = err
			continue
		}
		blog.Info("Removed dangling image %s", string(image))
	}
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
			except.Error(err, ": Error processing image", string(imageID))
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
