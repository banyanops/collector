// imagedata.go has funcitons that deal with the contents of images, including Linux distribution
// identification and application package names, versions, and architectures.
package collector

import (
	"encoding/json"
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

// Registry V2 authorization server result
type authServerResult struct {
	Token string `json:"token"`
}

// PullImage performs a docker pull on an image specified by repo/tag.
func PullImage(metadata *ImageMetadataInfo) (err error) {
	tagspec := metadata.Repo + ":" + metadata.Tag
	if RegistrySpec != config.DockerHub {
		tagspec = RegistrySpec + "/" + tagspec
	}
	apipath := "/images/create?fromImage=" + tagspec
	blog.Info("PullImage downloading %s, Image ID: %s", apipath, metadata.Image)
	config.BanyanUpdate("Pull", apipath, metadata.Image)
	resp, err := DockerAPI(DockerTransport, "POST", apipath, []byte{}, XRegistryAuth)
	if err != nil {
		except.Error(err, "PullImage failed for", RegistrySpec, metadata.Repo, metadata.Tag, metadata.Image)
		return
	}
	if strings.Contains(string(resp), `"error":`) {
		err = errors.New("PullImage error for " + RegistrySpec + "/" + metadata.Repo + "/" + metadata.Tag)
		except.Error(err)
		return
	}
	blog.Trace(string(resp))

	// get the Docker-calculated image ID
	calculatedID, err := dockerImageID(RegistrySpec, metadata)
	if err != nil {
		except.Error(err, "dockerImageID")
		return
	}
	if metadata.Image > "" && metadata.Image != calculatedID {
		newMetadata := *metadata
		newMetadata.Image = calculatedID
		RemoveImages([]ImageMetadataInfo{newMetadata})
		err = errors.New("PullImage " + metadata.Repo + ":" + metadata.Tag +
			" image ID " + calculatedID + " doesn't match metadata-derived ID " +
			metadata.Image)
		except.Error(err)
		return err
	}
	metadata.Image = calculatedID
	return
}

func dockerImageID(regspec string, metadata *ImageMetadataInfo) (ID string, err error) {
	matchRepo := string(metadata.Repo)
	if regspec != config.DockerHub {
		matchRepo = regspec + "/" + matchRepo
	}
	matchTag := string(metadata.Tag)
	if strings.HasPrefix(matchRepo, "library/") {
		matchRepo = strings.Replace(matchRepo, "library/", "", 1)
	}
	// verify the image ID of the pulled image matches the expected metadata.
	imageMap, err := GetLocalImages(false, false)
	if err != nil {
		except.Error(err, ":unable to list local images")
		return
	}
	for imageID, repotagSlice := range imageMap {
		for _, repotag := range repotagSlice {
			if string(repotag.Repo) == matchRepo && string(repotag.Tag) == matchTag {
				ID = string(imageID)
				return
			}
		}
	}
	err = errors.New("Failed to find local image ID for " + metadata.Repo + ":" + metadata.Tag)
	except.Error(err)
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

// RegistryQueryV1 performs an HTTP GET operation from a V1 registry and returns the response.
func RegistryQueryV1(client *http.Client, URL string) (response []byte, e error) {
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

// RegistryQueryV2 performs an HTTP GET operation from the registry and returns the response.
// If the initial response code is 401 Unauthorized, then this function issues a call
// if indicated by an WWW-Authenticate header in the response to get a token, and
// then re-issues the initial call to get the final response.
func RegistryQueryV2(client *http.Client, URL string) (response []byte, e error) {
	_, _, BasicAuth, XRegistryAuth = GetRegistryURL()
	req, e := http.NewRequest("GET", URL, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Basic "+BasicAuth)
	r, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	if r.StatusCode == 401 {
		blog.Debug("Registry Query %s got 401", URL)
		// get the WWW-Authenticate header
		WWWAuth := r.Header.Get("WWW-Authenticate")
		if WWWAuth == "" {
			except.Error("Empty WWW-Authenticate", URL)
			return
		}
		arr := strings.Fields(WWWAuth)
		if len(arr) != 2 {
			e = errors.New("Invalid WWW-Authenticate format for " + WWWAuth)
			except.Error(e)
			return
		}
		authType := arr[0]
		blog.Debug("Authorization type: %s", authType)
		fieldMap := make(map[string]string)
		e = parseAuthenticateFields(arr[1], fieldMap)
		if e != nil {
			except.Error(e)
			return
		}
		r.Body.Close()
		// access the authentication server to get a token
		token, err := queryAuthServerV2(client, fieldMap, BasicAuth)
		if err != nil {
			except.Error(err)
			return nil, err
		}
		// re-issue the original request, this time using the token
		req, e = http.NewRequest("GET", URL, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Authorization", authType+" "+token)
		r, e = client.Do(req)
		if e != nil {
			return nil, e
		}
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

/* queryAuthServerV2 retrieves an authorization token from a V2 auth server */
func queryAuthServerV2(client *http.Client, fieldMap map[string]string, BasicAuth string) (token string, e error) {
	authServer := fieldMap["realm"]
	if authServer == "" {
		e = errors.New("No registry token auth server specified")
		return
	}
	blog.Debug("authServer=%s\n", authServer)
	URL := authServer
	first := true
	for key, value := range fieldMap {
		if key != "realm" {
			if first {
				URL = URL + "?"
				first = false
			} else {
				URL = URL + "&"
			}
			URL = URL + key + "=" + value
		}
	}
	blog.Debug("Auth server URL is %s", URL)

	req, e := http.NewRequest("GET", URL, nil)
	if e != nil {
		return
	}
	req.Header.Set("Authorization", "Basic "+BasicAuth)
	r, e := client.Do(req)
	if e != nil {
		return
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e := ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	var parsedReply authServerResult
	e = json.Unmarshal(response, &parsedReply)
	if e != nil {
		return
	}
	token = parsedReply.Token
	return token, e
}

func parseAuthenticateFields(s string, fieldMap map[string]string) (e error) {
	fields := strings.Split(s, ",")
	for _, f := range fields {
		arr := strings.Split(f, "=")
		if len(arr) != 2 {
			e = errors.New("Invalid WWW-Auth field format for " + f)
			return
		}
		key := arr[0]
		value := strings.Replace(arr[1], `"`, "", -1)
		fieldMap[key] = value
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
