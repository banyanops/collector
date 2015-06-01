// metadata.go has functions to gather and save metadata about a Docker image, including its ID, Author, Parent,
// Creation time, etc.
package collector

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
)

var (
	ReposToProcess = make(map[RepoType]bool)
	ExcludeRepo    = func() map[RepoType]bool {
		excludeList := []RepoType{} // You can add repos to this list
		m := make(map[RepoType]bool)
		for _, r := range excludeList {
			m[r] = true
		}
		return m
	}()
)

// ImageSet is a set of image IDs.
type ImageSet map[ImageIDType]bool

// NewImageSet creates a new ImageSet.
func NewImageSet() ImageSet {
	return ImageSet(make(map[ImageIDType]bool))
}

// HubInfo records the index and auth information provided by Docker Hub to access a repository.
type HubInfo struct {
	Repo        RepoType
	DockerToken string
	RegistryURL string
}

// ImageMetadataInfo records basic information about an image.
type ImageMetadataInfo struct {
	Image    string    //this has to be the first field (used in order by)
	Datetime time.Time //created at
	Repo     string
	Tag      string
	Size     uint64
	Author   string
	Checksum string
	Comment  string
	Parent   string
}

// ImiSet is a set of Image Metadata Info structures.
type ImiSet map[ImageMetadataInfo]bool

// ImageIMIMap maps image IDs to ImageMetadataInfo structs.
type ImageIMIMap map[ImageIDType]ImageMetadataInfo

// NewImiSet creates a new ImiSet.
func NewImiSet() ImiSet {
	return ImiSet(make(map[ImageMetadataInfo]bool))
}

// NewImageIMIMap is a constructor for ImageIMIMap.
func NewImageIMIMap() ImageIMIMap {
	return make(map[ImageIDType]ImageMetadataInfo)
}

// Insert adds an image ID to an Image ID Map.
func (iim ImageIMIMap) Insert(imageID ImageIDType, imi ImageMetadataInfo) {
	iim[imageID] = imi
}

// Exists checks whether an image ID is present in an ImageIMIMap.
func (iim ImageIMIMap) Exists(imageID ImageIDType) bool {
	_, ok := iim[imageID]
	return ok
}

// Imi returns the ImageMetadataInfo corresponding to an image ID if that image
// is present in the input ImageIMIMap.
func (iim ImageIMIMap) Imi(imageID ImageIDType) (val ImageMetadataInfo, e error) {
	val, ok := iim[imageID]
	if ok {
		return
	}
	e = errors.New("Image " + string(imageID) + " is not in the Image Metadata Map")
	return
}

// TagType represents docker repository tags.
type TagType string

// RepoType represents docker repositories.
type RepoType string

// ImageIDType represents docker image IDs.
type ImageIDType string

// TagInfo records the tag-to-image mappings for a single Docker repository.
type TagInfo struct {
	Repo   RepoType
	TagMap map[TagType]ImageIDType
}

// RepoTagType represents a docker repository and tag.
type RepoTagType struct {
	Repo RepoType
	Tag  TagType
}

// Docker repository description.
type repo struct {
	Description string
	Name        string
}

// Docker registry search reply
type registrySearchResult struct {
	NumResults int    `json:"num_results"`
	Query      string `json:"query"`
	Results    []repo
}

// imageStruct records information returned by the registry to describe an image.
// This information gets copied to an object of type ImageMetadataInfo.
type imageStruct struct {
	ID       string
	Parent   string
	Checksum string
	Created  string
	// Container string
	Author  string
	Size    uint64
	Comment string
}

// HubInfoMap maps repository name to the corresponding Docker Hub auth/index info.
type HubInfoMap map[RepoType]HubInfo

// NewHubInfoMap is a constructor for HubInfoMap.
func NewHubInfoMap() HubInfoMap {
	return make(map[RepoType]HubInfo)
}

// ByDateTime is used to sort ImageMetadataInfo slices by image age from newest to oldest.
type ByDateTime []ImageMetadataInfo

func (a ByDateTime) Len() int {
	return len(a)
}
func (a ByDateTime) Swap(i int, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a ByDateTime) Less(i int, j int) bool {
	return a[i].Datetime.After(a[j].Datetime)
}

// GetImageToMDMap takes image metadata structs and produces a map of imageID to metadata struct.
func GetImageToMDMap(imageMDs []ImageMetadataInfo) (imageToMDMap map[string][]ImageMetadataInfo) {
	imageToMDMap = make(map[string][]ImageMetadataInfo)
	for _, imageMD := range imageMDs {
		imageToMDMap[imageMD.Image] = append(imageToMDMap[imageMD.Image], imageMD)
	}
	return
}

// GetImageMetadata returns repository/tag/image metadata queried from a Docker registry.
// If the user has specified the repositories to examine, then no other repositories are examined.
// If the user has not specified repositories, then the registry search API is used to
// get the list of all repositories in the registry.
func GetImageMetadata(oldImiSet ImiSet) (tagSlice []TagInfo, imi []ImageMetadataInfo) {
	for {
		blog.Info("Get Repos")
		repoSlice, e := getRepos()
		if e != nil {
			blog.Warn(e, " getRepos")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}

		blog.Info("Get Tags")
		// Now get a list of all the tags
		tagSlice, e = getTags(repoSlice)
		if e != nil {
			blog.Warn(e, " getTags")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}

		blog.Info("Get Image Metadata")
		// Get image metadata
		imi, e = getImageMetadata(tagSlice, oldImiSet)
		if e != nil {
			blog.Warn(e, " getImageMetadata")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}
		break
	}

	return
}

// GetImageMetadataHub returns repositories/tags/image metadata from the Docker Hub.
// The user must have specified a set of repositories of interest.
// The function queries Docker Hub as an index to the registries, and then retrieves
// information directly from the registries, using Docker Hub authentication tokens.
func GetImageMetadataHub(oldImiSet ImiSet) (tagSlice []TagInfo, imi []ImageMetadataInfo) {
	for {
		blog.Info("Get Repos from Docker Hub")
		hubInfoSlice, e := getReposHub()
		if e != nil {
			blog.Warn(e, " getReposHub")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}

		blog.Info("Get Tags and Metadata from Docker Hub")
		// Now get a list of all the tags
		tagSlice, imi, e = getTagsMetadataHub(hubInfoSlice, oldImiSet)
		if e != nil {
			blog.Warn(e, " getTagsMetadataHub")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}
		break
	}

	return
}

// getRepos queries the Docker registry for the list of the repositories it is currently hosting.
// However, if the user specified a list of repositories, then getRepos() just returns that list
// of specified repositories and does not query the Docker registry.
func getRepos() (repoSlice []RepoType, err error) {
	if len(ReposToProcess) > 0 {
		for repo := range ReposToProcess {
			repoSlice = append(repoSlice, repo)
		}
		return
	}

	// a query with an empty query string returns all the repos
	r, err := http.Get(RegistryAPIURL + "/v1/search?q=")
	if err != nil {
		return
	}
	defer r.Body.Close()
	response, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}
	// parse the JSON response body and populate repo slice
	var result registrySearchResult
	if err = json.Unmarshal(response, &result); err != nil {
		return
	}
	for _, elem := range result.Results {
		if ExcludeRepo[RepoType(elem.Name)] {
			continue
		}
		repoSlice = append(repoSlice, RepoType(elem.Name))
	}
	return
}

// getReposHub validates the user-specified list of repositories against the Docker Hub index.
// It returns a list of HubInfo structs with index info for each validated repository.
func getReposHub() (hubInfo []HubInfo, err error) {
	// lookup defines a function that takes a repository name as input and returns
	// the Docker auth token and registry URL to access that repository.
	lookup := func(repo RepoType) (dockerToken, registryURL string) {
		client := &http.Client{}
		URL := RegistryAPIURL + "/v1/repositories/" + string(repo) + "/images"
		req, e := http.NewRequest("GET", URL, nil)
		req.Header.Set("X-Docker-Token", "true")
		r, e := client.Do(req)
		if e != nil {
			blog.Error(e, ":getReposHub HTTP request failed")
			return
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			blog.Error("getReposHub HTTP bad status code %d from Docker Hub", r.StatusCode)
			return
		}
		dockerToken = r.Header.Get("X-Docker-Token")
		registryURL = r.Header.Get("X-Docker-Endpoints")
		arr := strings.Split(registryURL, ",")
		if len(arr) == 0 {
			registryURL = ""
			return
		}
		registryURL = strings.TrimSpace(arr[0])
		return
	}
	if len(ReposToProcess) > 0 {
		for repo := range ReposToProcess {
			dockerToken, registryURL := lookup(repo)
			if dockerToken == "" {
				blog.Error(repo, ":Could not find info for repo.")
				continue
			}
			hubInfo = append(hubInfo,
				HubInfo{Repo: repo, DockerToken: dockerToken, RegistryURL: registryURL})
		}
	}
	return
}

// getTags queries the Docker registry for the list of the tags for each repository.
func getTags(repoSlice []RepoType) (tagSlice []TagInfo, e error) {
	for _, repo := range repoSlice {
		// get tags for one repo
		r, e := http.Get(RegistryAPIURL + "/v1/repositories/" + string(repo) + "/tags")
		if e != nil {
			return nil, e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			blog.Error("Skipping Repo:", repo, "tag lookup status code:", r.StatusCode)
			continue
		}
		response, e := ioutil.ReadAll(r.Body)
		blog.Debug(string(response))
		if e != nil {
			return nil, e
		}
		//parse JSON output
		var m map[TagType]ImageIDType
		if e = json.Unmarshal(response, &m); e != nil {
			return nil, e
		}
		var t TagInfo
		t.Repo = repo
		t.TagMap = m
		tagSlice = append(tagSlice, t)
	}
	return
}

// getTagsMetadataHub takes Docker Hub auth and index info and uses it to query
// registries for the tags and metadata for each repository.
func getTagsMetadataHub(hubInfoSlice []HubInfo, oldImiSet ImiSet) (tagSlice []TagInfo,
	imi []ImageMetadataInfo, e error) {

	// populate map from ImageID to HubInfo (docker hub token)
	hubInfoMap := NewHubInfoMap()
	for _, h := range hubInfoSlice {
		hubInfoMap[h.Repo] = h
	}

	// populate map from ImageID to Image Metadata Info
	imimap := NewImageIMIMap()
	previousImages := NewImageSet()
	for imi := range oldImiSet {
		imimap.Insert(ImageIDType(imi.Image), imi)
		previousImages[ImageIDType(imi.Image)] = true
	}

	// get tag and image metadata info
	for _, hubInfo := range hubInfoSlice {
		// singleTagSlice: get all the tags for a single repo
		var singleTagSlice []TagInfo
		singleTagSlice, e = lookupTagsHub(hubInfo)
		if e != nil {
			blog.Error(e, ": Error in looking up tags in dockerhub")
			//ignore this repo and continue  (changed from return to continue)
			//TODO: Make sure that this fix has no other side effects
			continue
		}
		tagSlice = append(tagSlice, singleTagSlice...)

		ch := make(chan ImageMetadataInfo)
		errch := make(chan error)
		goCount := 0
		// for each tag, generate the current Image Metadata Info
		for _, repotag := range singleTagSlice {
			repo := repotag.Repo
			tagmap := repotag.TagMap
			for tag, imageID := range tagmap {
				var curr ImageMetadataInfo
				if imimap.Exists(imageID) {
					// copy previous entry and fill in this repo/tag
					curr, _ = imimap.Imi(imageID)
					curr.Repo = string(repo)
					curr.Tag = string(tag)
					imi = append(imi, curr)
				} else {
					// create a new entry, and determine field values
					// by querying the registry
					goCount++
					go func(repo RepoType, tag TagType, imageID ImageIDType, hubInfo HubInfo,
						ch chan ImageMetadataInfo, errch chan error) {
						var metadata ImageMetadataInfo
						metadata, e = lookupMetadataHub(repo, tag, imageID, hubInfo)
						if e != nil {
							blog.Error(e, "Unable to lookup metadata for",
								repo, ":", tag, string(imageID))
							//ignore this metadata and move on (changed from return to continue)
							//TODO: Make sure that this fix has no other side effects
							errch <- e
							return
						}
						ch <- metadata
					}(repo, tag, imageID, hubInfo, ch, errch)
				}
			}
		}
		for i := 0; i < goCount; i++ {
			select {
			case metadata := <-ch:
				imi = append(imi, metadata)
			case <-errch:
				continue
				// blog.Error(err, ":getImageMetadata")
			}
		}
	}
	return
}

// lookupTagsHub accesses the registries pointed to by Docker Hub and returns tag and image info
// for each specified repository.
func lookupTagsHub(info HubInfo) (tagSlice []TagInfo, e error) {
	client := &http.Client{}
	URL := "https://" + info.RegistryURL + "/v1/repositories/" + string(info.Repo) + "/tags"
	//log.Print(URL)
	var req *http.Request
	req, e = http.NewRequest("GET", URL, nil)
	req.Header.Set("Authorization", "Token "+info.DockerToken)
	var r *http.Response
	r, e = client.Do(req)
	if e != nil {
		return
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		e = errors.New("Skipping Repo: " + string(info.Repo) + "tag lookup status code:" +
			strconv.Itoa(r.StatusCode))
		return
	}
	var response []byte
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	//parse JSON output
	var m map[TagType]ImageIDType
	if e = json.Unmarshal(response, &m); e != nil {
		return nil, e
	}
	var t TagInfo
	t.Repo = info.Repo
	t.TagMap = m
	tagSlice = append(tagSlice, t)
	return
}

// lookupMetadataHub takes as input matching repo, tag, imageID, and Docker Hub auth/index info,
// and it returns ImageMetadataInfo for that image by querying the indexed registry.
func lookupMetadataHub(repo RepoType, tag TagType, imageID ImageIDType, hubInfo HubInfo) (
	imi ImageMetadataInfo, e error) {

	blog.Info("Get Metadata for Image: %s", string(imageID))
	client := &http.Client{}
	var req *http.Request
	URL := "https://" + hubInfo.RegistryURL + "/v1/images/" + string(imageID) + "/json"
	req, e = http.NewRequest("GET", URL, nil)
	// log.Print("metadata query to: ", URL)
	tokenString := "Token " + hubInfo.DockerToken
	req.Header.Set("Authorization", tokenString)
	// log.Print("Authorization:", tokenString)
	var r *http.Response
	r, e = client.Do(req)
	if e != nil {
		return
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		e = errors.New("Unable to query metadata for Repo: " + string(repo) +
			"Tag: " + string(tag) + " Image: " + string(imageID))
		return
	}
	var response []byte
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	// log.Print("metadata query response: " + string(response))
	var m imageStruct
	if e = json.Unmarshal(response, &m); e != nil {
		return
	}
	var creationTime time.Time
	imi.Image = string(imageID)
	if creationTime, e = time.Parse(time.RFC3339Nano, m.Created); e != nil {
		return
	}
	imi.Datetime = creationTime
	imi.Repo = string(repo)
	imi.Tag = string(tag)
	imi.Size = m.Size
	imi.Author = m.Author
	imi.Checksum = m.Checksum
	imi.Comment = m.Comment
	imi.Parent = m.Parent
	return
}

// GetNewImageMetadata takes the set of existing images, queries the registry to find any changes,
// and then brings the Output Writer up to date by telling it the obsolete metadata to delete
// and the new metadata to add.
func GetNewImageMetadata(oldImiSet ImiSet) (tagSlice []TagInfo,
	imi []ImageMetadataInfo, currentImiSet ImiSet) {

	var currentImi []ImageMetadataInfo
	switch {
	case HubAPI == false:
		tagSlice, currentImi = GetImageMetadata(oldImiSet)
	case HubAPI == true:
		tagSlice, currentImi = GetImageMetadataHub(oldImiSet)
	}

	// get only the new IMIs from currentImi
	currentImiSet = NewImiSet()
	for _, metadata := range currentImi {
		currentImiSet[metadata] = true
		if _, ok := oldImiSet[metadata]; !ok {
			// metadata is not in old map
			imi = append(imi, metadata)
		}
	}

	// find entries in the old map that are not in the current map,
	// and remove those entries from the database
	obsolete := []ImageMetadataInfo{}
	for metadata := range oldImiSet {
		if _, ok := currentImiSet[metadata]; !ok {
			if len(ReposToProcess) > 0 {
				if _, present := ReposToProcess[RepoType(metadata.Repo)]; present {
					obsolete = append(obsolete, metadata)
					blog.Info("Need to remove ImageMetadata: %v", metadata)
				}
			} else {
				obsolete = append(obsolete, metadata)
				blog.Info("Need to remove ImageMetadata: %v", metadata)
			}
		}
	}
	if len(obsolete) > 0 {
		RemoveObsoleteMetadata(obsolete)
	}

	// Sort image metadata from newest image to oldest image
	sort.Sort(ByDateTime(imi))
	return
}

// RemoveObsoleteMetadata removes obsolete metadata from the Banyan service.
func RemoveObsoleteMetadata(obsolete []ImageMetadataInfo) {
	if len(obsolete) == 0 {
		blog.Warn("No image metadata to save!")
		return
	}

	for _, writer := range WriterList {
		writer.RemoveImageMetadata(obsolete)
	}

	return
}

// getImageMetadata queries the Docker registry for info about each image.
func getImageMetadata(tagSlice []TagInfo, oldImiSet ImiSet) (imi []ImageMetadataInfo, e error) {

	imimap := NewImageIMIMap()
	previousImages := NewImageSet()
	for imi := range oldImiSet {
		imimap.Insert(ImageIDType(imi.Image), imi)
		previousImages[ImageIDType(imi.Image)] = true
	}

	// get map from each imageID to all of its aliases (repo+tag)
	imageMap := make(map[ImageIDType][]RepoTagType)
	for _, ti := range tagSlice {
		for tag, imageID := range ti.TagMap {
			repotag := RepoTagType{Repo: ti.Repo, Tag: tag}

			if _, ok := imageMap[imageID]; ok {
				imageMap[imageID] = append(imageMap[imageID], repotag)
			} else {
				imageMap[imageID] = []RepoTagType{repotag}
			}
		}
	}

	// for each alias, create an entry in imi
	ch := make(chan ImageMetadataInfo)
	errch := make(chan error)
	goCount := 0
	for imageID := range imageMap {
		var curr ImageMetadataInfo
		if previousImages[imageID] {
			// We already know this image's metadata, but we need to record
			// its current repo:tag aliases.
			var e error
			curr, e = imimap.Imi(imageID)
			if e != nil {
				blog.Error(e, "imageID", string(imageID), "not in imimap")
				continue
			}
			imi = append(imi, curr)
			continue
		}

		goCount++
		go func(imageID ImageIDType, ch chan ImageMetadataInfo, errch chan error) {
			var metadata ImageMetadataInfo
			blog.Info("Get Metadata for Image: %s", string(imageID))
			response, e := doHTTPGet(RegistryAPIURL + "/v1/images/" + string(imageID) + "/json")
			if e != nil {
				errch <- e
				return
			}
			var m imageStruct
			if e = json.Unmarshal(response, &m); e != nil {
				errch <- e
				return
			}
			metadata.Image = string(imageID)
			if c, e := time.Parse(time.RFC3339Nano, m.Created); e != nil {
				errch <- e
				return
			} else {
				metadata.Datetime = c
				metadata.Size = m.Size
				metadata.Author = m.Author
				metadata.Checksum = m.Checksum
				metadata.Comment = m.Comment
				metadata.Parent = m.Parent
			}
			ch <- metadata
		}(imageID, ch, errch)
	}
	for i := 0; i < goCount; i++ {
		select {
		case metadata := <-ch:
			imi = append(imi, metadata)
		case err := <-errch:
			blog.Error(err, ":getImageMetadata")
		}
	}

	// fill in the repo and tag fields of imi, replicating entries for multiple aliases to an image
	finalImi := []ImageMetadataInfo{}
	for _, md := range imi {
		for _, repotag := range imageMap[ImageIDType(md.Image)] {
			newmd := md
			// fill in the repo and tag
			// _ = repotag
			newmd.Repo = string(repotag.Repo)
			newmd.Tag = string(repotag.Tag)
			finalImi = append(finalImi, newmd)
		}
	}
	imi = finalImi
	return
}

// SaveImageMetadata saves image metadata to selected storage location
// (standard output, Banyan service, etc.).
func SaveImageMetadata(imi []ImageMetadataInfo) {
	if len(imi) == 0 {
		blog.Warn("No image metadata to save!")
		return
	}

	for _, writer := range WriterList {
		writer.AppendImageMetadata(imi)
	}

	return
}
