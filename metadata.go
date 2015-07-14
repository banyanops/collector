// metadata.go has functions to gather and save metadata about a Docker image, including its ID, Author, Parent,
// Creation time, etc.
package collector

import (
	"bytes"
	"crypto/tls"
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

const (
	// maxGoCount imposes a limit on number of concurrent goroutines performing registry calls.
	maxGoCount = 10
	minGoCount = 5
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

func (is ImageSet) Insert(imageID ImageIDType) {
	is[imageID] = true
}

func (is ImageSet) Exists(imageID ImageIDType) bool {
	_, ok := is[imageID]
	return ok
}

// IndexInfo records the index and auth information provided by Docker Hub to access a repository.
type IndexInfo struct {
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

// MetadataSet is a set of Image Metadata Info structures.
type MetadataSet map[ImageMetadataInfo]bool

// ImageToMetadataMap maps image IDs to ImageMetadataInfo structs.
type ImageToMetadataMap map[ImageIDType]ImageMetadataInfo

// NewMetadataSet creates a new MetadataSet.
func NewMetadataSet() MetadataSet {
	return MetadataSet(make(map[ImageMetadataInfo]bool))
}

// NewImageToMetadataMap is a constructor for ImageToMetadataMap.
func NewImageToMetadataMap(s MetadataSet) ImageToMetadataMap {
	m := make(map[ImageIDType]ImageMetadataInfo)
	for metadata := range s {
		ImageToMetadataMap(m).Insert(ImageIDType(metadata.Image), metadata)
	}
	return m
}

// Insert adds an image ID to an Image ID Map.
func (m ImageToMetadataMap) Insert(imageID ImageIDType, metadata ImageMetadataInfo) {
	m[imageID] = metadata
}

// Exists checks whether an image ID is present in an ImageToMetadataMap.
func (m ImageToMetadataMap) Exists(imageID ImageIDType) bool {
	_, ok := m[imageID]
	return ok
}

// Metadata returns the ImageMetadataInfo corresponding to an image ID if that image
// is present in the input ImageToMetadataMap.
func (m ImageToMetadataMap) Metadata(imageID ImageIDType) (metadata ImageMetadataInfo, e error) {
	metadata, ok := m[imageID]
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

// ImageStruct records information returned by the registry to describe an image.
// This information gets copied to an object of type ImageMetadataInfo.
type ImageStruct struct {
	ID       string
	Parent   string
	Checksum string
	Created  string
	// Container string
	Author  string
	Size    uint64
	Comment string
}

// LocalImageStruct records information returned by the local daemon to describe an image,ff
// in a response to List Image query.
type LocalImageStruct struct {
	ID       string 	`json:"Id"`
	Parent   string		`json:"ParentId"`
	RepoTags []string	`json:"RepoTags"`
}

// IndexInfoMap maps repository name to the corresponding Docker Hub auth/index info.
type IndexInfoMap map[RepoType]IndexInfo

// NewIndexInfoMap is a constructor for IndexInfoMap.
func NewIndexInfoMap() IndexInfoMap {
	return make(map[RepoType]IndexInfo)
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

// getLocalImages queries the local Docker daemon for list of images.
func getLocalImages() (imageMap map[ImageIDType][]RepoTagType, e error) {

	// query a list of images from Docker daemon
	response, e := listImages()
	if e != nil {
		return nil, e
	}
	blog.Info(string(response))
	// parse JSON
	var localImageList []LocalImageStruct
	if e = json.Unmarshal(response, &localImageList); e != nil {
		return nil, e
	}

	// make map from each imageID to all of its aliases (repo+tag)
	imageMap = make(map[ImageIDType][]RepoTagType)
	for _, localImage := range localImageList {
		imageID := ImageIDType(localImage.ID)
		for _, repoTag := range localImage.RepoTags {
			// repoTag example: "localhost:5000/test/busybox:latest"
			// repo: "localhost:5000/test/busybox"
			// tag: "latest"
			ss := strings.Split(repoTag, ":")
			tag := ss[len(ss)-1]
			repo := repoTag[:len(repoTag)-len(tag)-1]
			blog.Debug(imageID, repoTag, repo, tag)

			repotag := RepoTagType{Repo: RepoType(repo), Tag: TagType(tag)}

			if _, ok := imageMap[imageID]; ok {
				imageMap[imageID] = append(imageMap[imageID], repotag)
			} else {
				imageMap[imageID] = []RepoTagType{repotag}
			}
		}
	}
	return
}

// GetLocalImageMetadata returns image metadata queried from a local Docker host.
// Query the local docker daemon to detect new image builds on the host and new images pulled from registry by users.
func GetLocalImageMetadata(oldMetadataSet MetadataSet) (metadataSlice []ImageMetadataInfo) {
	for {
		blog.Info("Get a list of images from local Docker daemon")
		imageMap, e := getLocalImages()
		if e != nil {
			blog.Warn(e, " getLocalImages")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}

		blog.Info("Get Image Metadata from local Docker daemon")
		// Get image metadata
		metadataSlice, e = getImageMetadata(imageMap, oldMetadataSet)
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

// GetImageMetadata returns repository/tag/image metadata queried from a Docker registry.
// If the user has specified the repositories to examine, then no other repositories are examined.
// If the user has not specified repositories, then the registry search API is used to
// get the list of all repositories in the registry.
func GetImageMetadata(oldMetadataSet MetadataSet) (tagSlice []TagInfo, metadataSlice []ImageMetadataInfo) {
	for {
		blog.Info("Get Repos")
		repoSlice, e := getRepos()
		if e != nil {
			blog.Warn(e, " getRepos")
			blog.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}
		if len(repoSlice) == 0 {
			// For some reason (like, registry search doesn't work), we are not
			// seeing any repos in the registry.
			// So, just reconstruct the list of repos that we saw earlier.
			blog.Warn("Empty repoSlice, reusing previous metadata")
			repomap := make(map[string]bool)
			for metadata := range oldMetadataSet {
				if repomap[metadata.Repo] == false {
					repoSlice = append(repoSlice, RepoType(metadata.Repo))
					repomap[metadata.Repo] = true
				}
			}
		}

		// Now get a list of all the tags, and the image metadata/manifest

		if *RegistryProto == "v1" {
			blog.Info("Get Tags")
			tagSlice, e = getTags(repoSlice)
			if e != nil {
				blog.Warn(e, " getTags")
				blog.Warn("Retrying")
				time.Sleep(config.RETRYDURATION)
				continue
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

			blog.Info("Get Image Metadata")
			// Get image metadata
			metadataSlice, e = getImageMetadata(imageMap, oldMetadataSet)
			if e != nil {
				blog.Warn(e, " getImageMetadata")
				blog.Warn("Retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			break
		}
		if *RegistryProto == "v2" {
			blog.Info("Get Tags and Metadata")
			tagSlice, metadataSlice, e = v2GetTagsMetadata(repoSlice)
			if e != nil {
				blog.Warn(e)
				blog.Warn("Retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			break
		}
	}

	return
}

// GetImageMetadataTokenAuthV1 returns repositories/tags/image metadata from the Docker Hub
// or other registry using v1 token authorization.
// The user must have specified a set of repositories of interest.
// The function queries the index server, e.g., Docker Hub, to get the token and registry, and then uses
// the token to query the registry.
func GetImageMetadataTokenAuthV1(oldMetadataSet MetadataSet) (tagSlice []TagInfo, metadataSlice []ImageMetadataInfo) {
	if len(ReposToProcess) == 0 {
		return
	}
	client := &http.Client{}

	metadataMap := NewImageToMetadataMap(oldMetadataSet)

	for repo := range ReposToProcess {
		blog.Info("Get index and tag info for %s", string(repo))
		config.BanyanUpdate("Get index and tag info for", string(repo))

		var (
			indexInfo         IndexInfo
			e                 error
			repoTagSlice      []TagInfo
			repoMetadataSlice []ImageMetadataInfo
		)

		// loop until success
		for {
			indexInfo, e = getReposTokenAuthV1(repo, client)
			if e != nil {
				blog.Warn(e, ":index lookup failed, retrying.")
				config.BanyanUpdate(e.Error(), ":index lookup failed, retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}

			repoTagSlice, e = getTagsTokenAuthV1(repo, client, indexInfo)
			if e != nil {
				blog.Warn(e, ":tag lookup failed, retrying.")
				config.BanyanUpdate(e.Error(), ":tag lookup failed, retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			if len(repoTagSlice) != 1 {
				blog.Error("Incorrect length of repoTagSlice: expected length=1, got length=%d", len(repoTagSlice))
				config.BanyanUpdate("Incorrect length of repoTagSlice")
				time.Sleep(config.RETRYDURATION)
				continue
			}

			repoMetadataSlice, e = getMetadataTokenAuthV1(repoTagSlice[0], metadataMap, client, indexInfo)
			if e != nil {
				blog.Warn(e, ":metadata lookup failed, retrying.")
				config.BanyanUpdate(e.Error(), ":metadata lookup failed, retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			//success!
			break
		}
		tagSlice = append(tagSlice, repoTagSlice...)
		metadataSlice = append(metadataSlice, repoMetadataSlice...)
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

	if *RegistryProto == "v2" {
		blog.Error("v2 registry search/catalog interface not yet supported in collector")
		return
	}

	// a query with an empty query string returns all the repos
	var client *http.Client
	if *RegistryTLSNoVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}
	response, err := RegistryQuery(client, RegistryAPIURL+"/v1/search?q=", BasicAuth)
	if err != nil {
		blog.Error(err)
		if s, ok := err.(*HTTPStatusCodeError); ok {
			blog.Error("HTTP bad status code %d from registry %s using --registryhttps=%v --registryauth=%v --registryproto=%s", s.StatusCode, RegistryAPIURL, *HTTPSRegistry, *AuthRegistry, *RegistryProto)
		}
		return
	}

	// parse the JSON response body and populate repo slice
	var result registrySearchResult
	if err = json.Unmarshal(response, &result); err != nil {
		blog.Error(err, "unmarshal", string(response))
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

// getReposTokenAuthV1 validates the user-specified list of repositories against an index server, e.g., Docker Hub.
// It returns a list of IndexInfo structs with index info for each validated repository.
func getReposTokenAuthV1(repo RepoType, client *http.Client) (indexInfo IndexInfo, e error) {
	// lookup defines a function that takes a repository name as input and returns
	// the Docker auth token and registry URL to access that repository.
	URL := RegistryAPIURL + "/v1/repositories/" + string(repo) + "/images"
	req, e := http.NewRequest("GET", URL, nil)
	req.Header.Set("X-Docker-Token", "true")
	if BasicAuth != "" {
		req.Header.Set("Authorization", "Basic "+BasicAuth)
	}
	r, e := client.Do(req)
	if e != nil {
		blog.Error(e, ":getReposTokenAuthV1 HTTP request failed")
		return
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	dockerToken := r.Header.Get("X-Docker-Token")
	if dockerToken == "" {
		e = errors.New("lookup error for repo " + string(repo))
		return
	}
	registryURL := r.Header.Get("X-Docker-Endpoints")
	arr := strings.Split(registryURL, ",")
	if len(arr) == 0 {
		registryURL = ""
		e = errors.New("lookup error for repo " + string(repo))
		return
	}
	registryURL = strings.TrimSpace(arr[0])
	indexInfo = IndexInfo{Repo: repo, DockerToken: dockerToken, RegistryURL: registryURL}
	return
}

func v1GetTags(repoSlice []RepoType) (tagSlice []TagInfo, e error) {
	var client *http.Client
	if *RegistryTLSNoVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}
	for _, repo := range repoSlice {
		// get tags for one repo
		response, err := RegistryQuery(client, RegistryAPIURL+"/v1/repositories/"+string(repo)+"/tags", BasicAuth)
		if err != nil {
			blog.Error(err)
			if s, ok := err.(*HTTPStatusCodeError); ok {
				blog.Error("Skipping Repo: %s, tag lookup status code %d", string(repo), s.StatusCode)
				continue
			}
			return
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

type V2Tag struct {
	Name string
	Tags []string
}

type V1Compat struct {
	V1Compatibility string
}
type V2Manifest struct {
	History []V1Compat
}

func v2GetMetadata(client *http.Client, repo, tag string) (metadata ImageMetadataInfo, e error) {
	response, err := RegistryQuery(client, RegistryAPIURL+"/v2/"+repo+"/manifests/"+tag, BasicAuth)
	if err != nil {
		blog.Error(err)
		if s, ok := err.(*HTTPStatusCodeError); ok {
			blog.Error("Skipping Repo: %s, tag lookup status code %d", string(repo), s.StatusCode)
			e = err
		}
		return
	}
	//parse JSON output
	var m V2Manifest
	b := bytes.NewBuffer(response)
	if e = json.NewDecoder(b).Decode(&m); e != nil {
		return
	}
	if len(m.History) == 0 {
		e = errors.New("repo " + repo + ":" + tag + " no images found in history")
		return
	}
	var image ImageStruct
	if e = json.Unmarshal([]byte(m.History[0].V1Compatibility), &image); e != nil {
		return
	}
	var creationTime time.Time
	metadata.Image = image.ID
	if creationTime, e = time.Parse(time.RFC3339Nano, image.Created); e != nil {
		return
	}
	metadata.Datetime = creationTime
	metadata.Repo = repo
	metadata.Tag = tag
	metadata.Size = image.Size
	metadata.Author = image.Author
	metadata.Checksum = image.Checksum
	metadata.Comment = image.Comment
	metadata.Parent = image.Parent
	return
}

// getTags queries the Docker registry for the list of the tags for each repository.
func getTags(repoSlice []RepoType) (tagSlice []TagInfo, e error) {
	switch *RegistryProto {
	case "v1", "quay":
		return v1GetTags(repoSlice)
	case "v2":
		panic("Unreachable")
	default:
		blog.Error("Unknown registry protocol %s", *RegistryProto)
		return
	}
	panic("Unreachable")
}

func getTagsTokenAuthV1(repo RepoType, client *http.Client, indexInfo IndexInfo) (tagSlice []TagInfo, e error) {
	tagSlice, e = lookupTagsTokenAuthV1(client, indexInfo)
	if e != nil {
		blog.Error(e, ": Error in looking up tags in dockerhub")
	}
	return
}

func getMetadataTokenAuthV1(repotag TagInfo, metadataMap ImageToMetadataMap, client *http.Client,
	indexInfo IndexInfo) (metadataSlice []ImageMetadataInfo, e error) {

	// for each tag, generate the current Image Metadata Info
	repo := repotag.Repo
	tagmap := repotag.TagMap
	for tag, imageID := range tagmap {
		if metadataMap.Exists(imageID) {
			continue
		}

		var metadata ImageMetadataInfo
		metadata, e = lookupMetadataTokenAuthV1(imageID, client, indexInfo)
		if e != nil {
			if s, ok := e.(*HTTPStatusCodeError); ok {
				blog.Error("Registry returned HTTP status code %d, skipping %s:%s image %s",
					s.StatusCode, string(repo), string(tag), string(imageID))
				continue
			}
			// some other error (network broken?), so give up
			blog.Error(e, "Unable to lookup metadata for",
				repo, ":", tag, string(imageID))
			return
		}
		metadata.Repo = string(repo)
		metadata.Tag = string(tag)
		metadataMap.Insert(ImageIDType(metadata.Image), metadata)
	}

	for tag, imageID := range tagmap {
		var curr ImageMetadataInfo
		if metadataMap.Exists(imageID) {
			// copy previous entry and fill in this repo/tag
			curr, _ = metadataMap.Metadata(imageID)
			curr.Repo = string(repo)
			curr.Tag = string(tag)
			metadataSlice = append(metadataSlice, curr)
		} else {
			e = errors.New("Missing metadata for image ID " + string(imageID))
			return
		}
	}
	return
}

// RegistryRequestWithToken queries a Docker Registry that uses v1 Token Auth, e.g., Docker Hub.
func RegistryRequestWithToken(client *http.Client, URL string, dockerToken string) (response []byte, e error) {
	var req *http.Request
	req, e = http.NewRequest("GET", URL, nil)
	if e != nil {
		blog.Error(e)
		return
	}
	req.Header.Set("Authorization", "Token "+dockerToken)
	var r *http.Response
	r, e = client.Do(req)
	if e != nil {
		blog.Error(e)
		return
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		blog.Error(e)
		return
	}
	return
}

// lookupTagsTokenAuthV1 accesses the registries pointed to by an index server, e.g., Docker Hub,
// and returns tag and image info for each specified repository.
func lookupTagsTokenAuthV1(client *http.Client, info IndexInfo) (tagSlice []TagInfo, e error) {
	URL := "https://" + info.RegistryURL + "/v1/repositories/" + string(info.Repo) + "/tags"
	response, e := RegistryRequestWithToken(client, URL, info.DockerToken)
	if e != nil {
		blog.Error(e)
		if s, ok := e.(*HTTPStatusCodeError); ok {
			e = errors.New("Skipping Repo: " + string(info.Repo) + "tag lookup status code:" +
				strconv.Itoa(s.StatusCode))
		}
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

// lookupMetadataTokenAuthV1 takes as input the imageID, and Docker Hub auth/index info,
// and it returns ImageMetadataInfo for that image by querying the indexed registry.
func lookupMetadataTokenAuthV1(imageID ImageIDType, client *http.Client, indexInfo IndexInfo) (
	metadata ImageMetadataInfo, e error) {

	blog.Info("Get Metadata for Image: %s", string(imageID))
	URL := "https://" + indexInfo.RegistryURL + "/v1/images/" + string(imageID) + "/json"
	response, e := RegistryRequestWithToken(client, URL, indexInfo.DockerToken)
	if e != nil {
		blog.Error(e, "Unable to query metadata for image: "+string(imageID))
		return
	}
	// log.Print("metadata query response: " + string(response))
	var m ImageStruct
	if e = json.Unmarshal(response, &m); e != nil {
		return
	}
	var creationTime time.Time
	metadata.Image = string(imageID)
	if creationTime, e = time.Parse(time.RFC3339Nano, m.Created); e != nil {
		return
	}
	metadata.Datetime = creationTime
	metadata.Size = m.Size
	metadata.Author = m.Author
	metadata.Checksum = m.Checksum
	metadata.Comment = m.Comment
	metadata.Parent = m.Parent
	return
}

// GetNewImageMetadata takes the set of existing images, queries the registry to find any changes,
// and then brings the Output Writer up to date by telling it the obsolete metadata to delete
// and the new metadata to add.
func GetNewImageMetadata(oldMetadataSet MetadataSet) (tagSlice []TagInfo,
	metadataSlice []ImageMetadataInfo, currentMetadataSet MetadataSet) {

	var currentMetadataSlice []ImageMetadataInfo
	//config.BanyanUpdate("Loading Registry Metadata")
	if LocalHost == true {
		blog.Info("Collect images from local Docker host")
		currentMetadataSlice = GetLocalImageMetadata(oldMetadataSet)
		// there is no tag API under Docker Remote API,
		// and the caller of GetNewImageMetadata ignores tagSlice
		tagSlice = nil
	} else {
		switch {
		case HubAPI == false:
			tagSlice, currentMetadataSlice = GetImageMetadata(oldMetadataSet)
		case HubAPI == true:
			tagSlice, currentMetadataSlice = GetImageMetadataTokenAuthV1(oldMetadataSet)
		}
	}


	// get only the new metadata from currentMetadataSlice
	currentMetadataSet = NewMetadataSet()
	for _, metadata := range currentMetadataSlice {
		currentMetadataSet[metadata] = true
		if _, ok := oldMetadataSet[metadata]; !ok {
			// metadata is not in old map
			metadataSlice = append(metadataSlice, metadata)
		}
	}

	// find entries in the old map that are not in the current map,
	// and remove those entries from the database
	obsolete := []ImageMetadataInfo{}
	for metadata := range oldMetadataSet {
		if _, ok := currentMetadataSet[metadata]; !ok {
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

	if len(metadataSlice) > 0 || len(obsolete) > 0 {
		config.BanyanUpdate("Detected changes in registry metadata")
	}

	// Sort image metadata from newest image to oldest image
	sort.Sort(ByDateTime(metadataSlice))
	return
}

const maxStatusLen = 100

func statusMessageMD(metadataSlice []ImageMetadataInfo) string {
	statString := ""
	for _, metadata := range metadataSlice {
		statString += metadata.Repo + ":" + metadata.Tag + ", "
		if len(statString) > maxStatusLen {
			return statString[0:maxStatusLen]
		}
	}
	return statString
}

// RemoveObsoleteMetadata removes obsolete metadata from the Banyan service.
func RemoveObsoleteMetadata(obsolete []ImageMetadataInfo) {
	if len(obsolete) == 0 {
		blog.Warn("No image metadata to save!")
		return
	}

	config.BanyanUpdate("Remove Metadata", statusMessageMD(obsolete))

	for _, writer := range WriterList {
		writer.RemoveImageMetadata(obsolete)
	}

	return
}

func v2GetTagsMetadata(repoSlice []RepoType) (tagSlice []TagInfo, metadataSlice []ImageMetadataInfo, e error) {
	var client *http.Client
	if *RegistryTLSNoVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}
	for _, repo := range repoSlice {
		// get tags for one repo
		response, err := RegistryQuery(client, RegistryAPIURL+"/v2/"+string(repo)+"/tags/list", BasicAuth)
		if err != nil {
			blog.Error(err)
			if s, ok := err.(*HTTPStatusCodeError); ok {
				blog.Error("Skipping Repo: %s, tag lookup status code %d", string(repo), s.StatusCode)
				continue
			}
			return
		}
		//parse JSON output
		var m V2Tag
		if e = json.Unmarshal(response, &m); e != nil {
			return
		}
		t := TagInfo{Repo: repo, TagMap: make(map[TagType]ImageIDType)}
		for _, tag := range m.Tags {
			metadata, e := v2GetMetadata(client, string(repo), tag)
			if e != nil {
				blog.Error(e, ":Unable to get metadata for repo", string(repo), "tag", tag)
				continue
			}
			t.TagMap[TagType(tag)] = ImageIDType(metadata.Image)
			metadataSlice = append(metadataSlice, metadata)
		}
		tagSlice = append(tagSlice, t)
	}
	return
}


// getImageMetadata queries the Docker registry for info about each image.
func getImageMetadata(imageMap map[ImageIDType][]RepoTagType,
	oldMetadataSet MetadataSet) (metadataSlice []ImageMetadataInfo, e error) {

	metadataMap := NewImageToMetadataMap(oldMetadataSet)
	previousImages := NewImageSet()
	for metadata := range oldMetadataSet {
		previousImages[ImageIDType(metadata.Image)] = true
	}

	// for each alias, create an entry in metadataSlice
	ch := make(chan ImageMetadataInfo)
	errch := make(chan error)
	goCount := 0
	var client *http.Client
	if *RegistryTLSNoVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}
	for imageID := range imageMap {
		var curr ImageMetadataInfo
		if previousImages[imageID] {
			// We already know this image's metadata, but we need to record
			// its current repo:tag aliases.
			var e error
			curr, e = metadataMap.Metadata(imageID)
			if e != nil {
				blog.Error(e, "imageID", string(imageID), "not in metadataMap")
				continue
			}
			metadataSlice = append(metadataSlice, curr)
			continue
		}

		goCount++
		go func(imageID ImageIDType, ch chan ImageMetadataInfo, errch chan error) {
			var metadata ImageMetadataInfo
			blog.Info("Get Metadata for Image: %s", string(imageID))

			var response []byte
			var e error

			if LocalHost {
				response, e = inspectImage(string(imageID))
				if e != nil {
					blog.Info(string(response))
				}
			} else {
				if *RegistryProto == "quay" {
					// TODO: Properly support quay.io image metadata instead of faking it.
					t := time.Date(2011, time.January, 1, 1, 0, 0, 0, time.UTC)
					metadata.Image = string(imageID)
					metadata.Datetime = t
					ch <- metadata
					return
				}
				response, e = RegistryQuery(client, RegistryAPIURL+"/v1/images/"+string(imageID)+"/json", BasicAuth)
			}

			if e != nil {
				errch <- e
				return
			}

			var m ImageStruct
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

		if goCount > maxGoCount {
			for ; goCount > minGoCount; goCount-- {
				select {
				case metadata := <-ch:
					metadataSlice = append(metadataSlice, metadata)
				case err := <-errch:
					blog.Error(err, ":getImageMetadata")
				}
			}
		}
	}
	for ; goCount > 0; goCount-- {
		select {
		case metadata := <-ch:
			metadataSlice = append(metadataSlice, metadata)
		case err := <-errch:
			blog.Error(err, ":getImageMetadata")
		}
	}

	// fill in the repo and tag fields of metadataSlice, replicating entries for multiple aliases to an image
	finalMetadataSlice := []ImageMetadataInfo{}
	for _, metadata := range metadataSlice {
		for _, repotag := range imageMap[ImageIDType(metadata.Image)] {
			newmd := metadata
			// fill in the repo and tag
			// _ = repotag
			newmd.Repo = string(repotag.Repo)
			newmd.Tag = string(repotag.Tag)
			finalMetadataSlice = append(finalMetadataSlice, newmd)
		}
	}
	metadataSlice = finalMetadataSlice
	return
}

// SaveImageMetadata saves image metadata to selected storage location
// (standard output, Banyan service, etc.).
func SaveImageMetadata(metadataSlice []ImageMetadataInfo) {
	if len(metadataSlice) == 0 {
		blog.Warn("No image metadata to save!")
		return
	}

	config.BanyanUpdate("Save Image Metadata", statusMessageMD(metadataSlice))

	for _, writer := range WriterList {
		writer.AppendImageMetadata(metadataSlice)
	}

	return
}

// ValidRepoName verifies that the name of a repo is in a legal format.
func ValidRepoName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if len(name) > 256 {
		blog.Error("Invalid repo name, too long: %s", name)
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
			continue
		case c >= 'A' && c <= 'Z':
			continue
		case c >= '0' && c <= '9':
			continue
		case c == '/' || c == '_' || c == '-' || c == '.':
			continue
		default:
			blog.Error("Invalid repo name %s", name)
			return false
		}
	}
	return true
}
