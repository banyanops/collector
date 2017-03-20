// metadata.go has functions to gather and save metadata about a Docker image, including its ID, Author, Parent,
// Creation time, etc.
package collector

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
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
	if len(imageID) > 0 {
		is[imageID] = true
	}
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
	OtherMetadata
	ManifestHash string // we calculate a sha256 hex of JSON image manifest returned by a v2 registry
	Registry     string
}

type OtherMetadata struct {
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

// Insert adds a metadata entry to a MetadataSet.
func (m MetadataSet) Insert(metadata ImageMetadataInfo) {
	m[metadata] = true
}

// cleanImageID returns an image ID after stripping any "encoding:" prefix.
func cleanImageID(ID string) string {
	if len(ID) == 0 {
		return ""
	}
	index := strings.LastIndex(ID, ":")
	if index < 0 {
		return ID
	}
	return ID[index+1:]
}

// cleanOther returns OtherMetadata with Parent image ID obtained by stripping any "encoding:" prefix
func cleanOther(other OtherMetadata) OtherMetadata {
	other.Parent = cleanImageID(other.Parent)
	return other
}

// Exists returns true if the metadata entry is in the MetadataSet.
// If there's no exact match, then a search is attempted for an
// entry in the set with non-contradictory values for
// Image and Metadata Hash (empty values are not
// considered contradictions) and match in all other fields.
func (m MetadataSet) Exists(metadata ImageMetadataInfo) bool {
	_, ok := m[metadata]
	if ok {
		return true
	}
	// No exact match, so search all items in the set for a Repo+Tag match
	// without a contradictory ManifestHash or imageID value
	metadataOther := cleanOther(metadata.OtherMetadata)
	for item, _ := range m {
		itemOther := cleanOther(item.OtherMetadata)
		if itemOther == metadataOther {
			time1 := item.Datetime.Truncate(time.Second)
			time2 := metadata.Datetime.Truncate(time.Second)
			if !time1.Equal(time2) {
				continue
			}
			contradiction := len(metadata.Image) > 0 && len(item.Image) > 0 &&
				cleanImageID(metadata.Image) != cleanImageID(item.Image)
			if contradiction {
				continue
			}
			contradiction = len(metadata.ManifestHash) > 0 && len(item.ManifestHash) > 0 &&
				metadata.ManifestHash != item.ManifestHash
			if contradiction {
				continue
			}
			// no contradiction for Image or ManifestHash
			return true
		}
	}
	return false
}

// SameRepoTag returns metadata entries from MetadataSet with the same repo & tag as metadata.
func (m MetadataSet) SameRepoTag(metadata ImageMetadataInfo) (matches []ImageMetadataInfo) {
	for item, _ := range m {
		if item.Repo == metadata.Repo && item.Tag == metadata.Tag {
			matches = append(matches, item)
		}
	}
	return
}

// Delete removes the metadata entry from the MetadataSet.
func (m MetadataSet) Delete(metadata ImageMetadataInfo) {
	_, ok := m[metadata]
	if ok {
		delete(m, metadata)
		return
	}
	for item, _ := range m {
		if len(item.ManifestHash) > 0 {
			if item.Repo == metadata.Repo && item.Tag == metadata.Tag &&
				item.ManifestHash == metadata.ManifestHash {
				delete(m, item)
				return
			}
		}
	}
}

// Replace removes matching metadata entry and replaces it with the updated version
func (m MetadataSet) Replace(metadata ImageMetadataInfo) {
	m.Delete(metadata)
	m.Insert(metadata)
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
	Repo     RepoType
	Tag      TagType
	Registry string
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
	ID       string   `json:"Id"`
	Parent   string   `json:"ParentId"`
	RepoTags []string `json:"RepoTags"`
}

// IndexInfoMap maps repository name to the corresponding Docker Hub auth/index info.
type IndexInfoMap map[RepoType]IndexInfo

// NewIndexInfoMap is a constructor for IndexInfoMap.
func NewIndexInfoMap() IndexInfoMap {
	return make(map[RepoType]IndexInfo)
}

// ImageToRepoTagMap maps image ID to all of its aliases (Repository+Tag).
type ImageToRepoTagMap map[ImageIDType][]RepoTagType

// Insert appends repotag to the slice of RepoTagTypes for imageID in ImageToRepoTagMap.
func (imageMap ImageToRepoTagMap) Insert(imageID ImageIDType, repotag RepoTagType) {
	if _, ok := imageMap[imageID]; ok {
		imageMap[imageID] = append(imageMap[imageID], repotag)
	} else {
		imageMap[imageID] = []RepoTagType{repotag}
	}
}

// RepoTags returns the repos and tags associated with imageID.
func (imageMap ImageToRepoTagMap) RepoTags(imageID ImageIDType) []RepoTagType {
	if repotag, ok := imageMap[imageID]; ok {
		return repotag
	}
	return []RepoTagType{}
}

// Image returns the imageID corresponding to a specified repo:tag.
func (imageMap ImageToRepoTagMap) Image(repo RepoType, tag TagType) (imageID ImageIDType, err error) {
	if strings.HasPrefix(string(repo), "library/") {
		repo = RepoType(strings.Replace(string(repo), "library/", "", 1))
	}
	for i, repotagSlice := range imageMap {
		for _, repotag := range repotagSlice {
			if repotag.Repo == repo && repotag.Tag == tag {
				imageID = i
				return
			}
		}
	}
	err = errors.New("Unable to find image ID for " + string(repo) + ":" + string(tag))
	return
}

// FilterRepoTag returns a new ImageToRepoTagMap that only has elements that match the given repo:tag.
func (imageMap ImageToRepoTagMap) FilterRepoTag(repotag RepoTagType) (newImageMap ImageToRepoTagMap) {
	newImageMap = make(ImageToRepoTagMap)
	for imageID, RepoTagSlice := range imageMap {
		for _, rt := range RepoTagSlice {
			if repotag == rt {
				newImageMap.Insert(imageID, rt)
			}
		}
	}
	return
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

// CheckRepoToProcess returns true if ReposToProcess is empty
// or if the given repo exists in ReposToProcess.
func CheckRepoToProcess(repo RepoType) bool {
	if len(ReposToProcess) == 0 {
		return true
	}
	// just check the presence of the key. Value doesn't matter.
	_, ok := ReposToProcess[repo]
	return ok
}

// repoTag info of local images queried from local Docker daemon may include a registry name
// example: "localhost:5000/test/busybox:latest" in
// "localhost:5000" is a registry
// "test/busybox" is a repository
// "latest" is a tag
// ExtractRepoTag conditionally strips out registry and returns registry, repo and tag in RepoTagType
func ExtractRepoTag(regRepoTag string, stripReg bool) (repoTag RepoTagType, e error) {

	ss := strings.Split(regRepoTag, ":")
	if len(ss) < 2 || len(ss) > 3 {
		e := errors.New("regRepoTag string has invalid format: " + regRepoTag)
		return repoTag, e
	}
	tag := ss[len(ss)-1]                               // the last component is a tag
	regRepo := regRepoTag[:len(regRepoTag)-len(tag)-1] // the remainder as registry + repository

	if !stripReg {
		repoTag = RepoTagType{Repo: RepoType(regRepo), Tag: TagType(tag)}
		return repoTag, e
	}

	ss = strings.Split(regRepo, "/")
	registry := ""
	if len(ss) > 1 && strings.ContainsAny(ss[0], ".:") {
		// ss[0] is a registry name, strip it out
		registry = ss[0]
		regRepo = regRepo[len(ss[0])+1:]
	}
	repoTag = RepoTagType{Registry: registry, Repo: RepoType(regRepo), Tag: TagType(tag)}
	return repoTag, e
}

// GetLocalImages queries the local Docker daemon for list of images.
// The registry name gets stripped from the repo nam if stripRegistry is set to true.
// The repo has to appear in the list of repos to check if checkRepo is set to true.
func GetLocalImages(stripRegistry bool, checkRepo bool) (imageMap ImageToRepoTagMap, e error) {

	// query a list of images from Docker daemon
	response, e := listImages()
	if e != nil {
		return nil, e
	}
	// parse JSON
	var localImageList []LocalImageStruct
	if e = json.Unmarshal(response, &localImageList); e != nil {
		return nil, e
	}

	// make map from each imageID to all of its aliases (repo+tag)
	imageMap = make(ImageToRepoTagMap)
	for _, localImage := range localImageList {
		imageID := ImageIDType(localImage.ID)
		for _, regRepoTag := range localImage.RepoTags {
			// skip images with no repo:tag
			if regRepoTag == "" || regRepoTag == "\u003cnone\u003e:\u003cnone\u003e" || regRepoTag == "<none>:<none>" {
				blog.Debug("Image %s has a <none>:<none> repository:tag.", string(imageID))
				continue
			}

			repoTag, e := ExtractRepoTag(regRepoTag, stripRegistry)
			if e != nil {
				return nil, e
			}

			if checkRepo {
				if CheckRepoToProcess(repoTag.Repo) {
					imageMap.Insert(imageID, repoTag)
				}
			} else {
				imageMap.Insert(imageID, repoTag)
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
		imageMap, e := GetLocalImages(true, true)
		if e != nil {
			except.Warn(e, " GetLocalImages")
			except.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}

		blog.Info("Get Image Metadata from local Docker daemon")
		// Get image metadata
		metadataSlice, e = GetImageMetadataSpecifiedV1(imageMap, oldMetadataSet)
		if e != nil {
			except.Warn(e, " GetImageMetadata")
			except.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}
		break
	}
	return
}

// GetImageMetadata determines which image metadata is of interest and then calls
// either GetImageMetadataSpecifiedV1 or v1GetTagsMetadata to obtain and return the appropriate metadata,
// depending on whether the info needs to come from a local Docker daemon or a V1 or V2 Docker registry.
// If the user has specified the repositories to examine, then no other repositories are examined.
// If the user has not specified repositories, then the registry search API is used to
// get the list of all repositories in the registry.
func GetImageMetadata(oldMetadataSet MetadataSet) (metadataSlice []ImageMetadataInfo) {
	tagSlice := []TagInfo{}
	for {
		blog.Info("Get Repos")
		repoSlice, e := getRepos()
		if e != nil {
			except.Warn(e, " getRepos")
			except.Warn("Retrying")
			time.Sleep(config.RETRYDURATION)
			continue
		}
		if len(repoSlice) == 0 {
			// For some reason (like, registry search doesn't work), we are not
			// seeing any repos in the registry.
			// So, just reconstruct the list of repos that we saw earlier.
			except.Warn("Empty repoSlice, reusing previous metadata")
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
				except.Warn(e, " getTags")
				except.Warn("Retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}

			// get map from each imageID to all of its aliases (repo+tag)
			imageMap := make(ImageToRepoTagMap)
			for _, ti := range tagSlice {
				for tag, imageID := range ti.TagMap {
					repotag := RepoTagType{Registry: RegistrySpec, Repo: ti.Repo, Tag: tag}

					imageMap.Insert(imageID, repotag)
				}
			}

			blog.Info("Get Image Metadata")
			// Get image metadata
			metadataSlice, e = GetImageMetadataSpecifiedV1(imageMap, oldMetadataSet)
			if e != nil {
				except.Warn(e, " GetImageMetadataSpecifiedV1")
				except.Warn("Retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			break
		}
		if *RegistryProto == "v2" {
			blog.Info("Get Tags and Metadata")
			metadataSlice, e = v2GetTagsMetadata(repoSlice)
			if e != nil {
				except.Warn(e)
				except.Warn("Retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			break
		}
	}

	return
}

// NeedRegistrySearch checks ReposToProcess and returns the search term to use
// if the registry search API needs to be invoked, else "".
func NeedRegistrySearch() (searchTerm string) {
	if len(ReposToProcess) != 1 {
		return ""
	}
	for repo, _ := range ReposToProcess {
		if strings.HasSuffix(string(repo), "*") {
			searchTerm := strings.Replace(string(repo), "*", "", 1)
			if strings.HasSuffix(searchTerm, "/") {
				searchTerm = searchTerm[:len(searchTerm)-1]
			}
			config.FilterRepos = false
			return searchTerm
		}
	}
	return ""
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

	allRepos := []RepoType{}
	// Check if we need to use the search API, i.e. only one repo given, and ends in wildcard "*".
	if searchTerm := NeedRegistrySearch(); searchTerm != "" {
		blog.Info("Using search API")
		var e error
		allRepos, e = registrySearchV1(client, searchTerm)
		if e != nil {
			except.Error(e, ":registry search")
			return
		}
	}
	// If search wasn't needed, the repos were individually specified.
	if len(allRepos) == 0 {
		for repo := range ReposToProcess {
			allRepos = append(allRepos, repo)
		}
	}

	for _, repo := range allRepos {
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
				except.Warn(e, ":index lookup failed for repo", string(repo), "- retrying.")
				config.BanyanUpdate(e.Error(), ":index lookup failed, repo", string(repo), "- retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}

			repoTagSlice, e = getTagsTokenAuthV1(repo, client, indexInfo)
			if e != nil {
				except.Warn(e, ":tag lookup failed for repo", string(repo), "- retrying.")
				config.BanyanUpdate(e.Error(), ":tag lookup failed for repo", string(repo), "- retrying")
				time.Sleep(config.RETRYDURATION)
				continue
			}
			if len(repoTagSlice) != 1 {
				except.Error("Incorrect length of repoTagSlice: expected length=1, got length=%d", len(repoTagSlice))
				config.BanyanUpdate("Incorrect length of repoTagSlice:", strconv.Itoa(len(repoTagSlice)), string(repo))
				time.Sleep(config.RETRYDURATION)
				continue
			}

			repoMetadataSlice, e = getMetadataTokenAuthV1(repoTagSlice[0], metadataMap, client, indexInfo)
			if e != nil {
				except.Warn(e, ":metadata lookup failed for", string(repoTagSlice[0].Repo), "- retrying.")
				config.BanyanUpdate(e.Error(), ":metadata lookup failed for", string(repoTagSlice[0].Repo), "- retrying")
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

// registrySearchV1 queries the Docker registry, returning a slice of repos.
func registrySearchV1(client *http.Client, searchTerm string) (repoSlice []RepoType, err error) {
	response, err := RegistryQueryV1(client, RegistryAPIURL+"/v1/search?q="+searchTerm)
	if err != nil {
		except.Error(err)
		if s, ok := err.(*HTTPStatusCodeError); ok {
			except.Error("HTTP bad status code %d from registry %s using --registryhttps=%v --registryauth=%v --registryproto=%s", s.StatusCode, RegistryAPIURL, *HTTPSRegistry, *AuthRegistry, *RegistryProto)
		}
		return
	}

	// parse the JSON response body and populate repo slice
	var result registrySearchResult
	if err = json.Unmarshal(response, &result); err != nil {
		except.Error(err, "unmarshal", string(response))
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
		except.Error("v2 registry search/catalog interface not yet supported in collector")
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
	return registrySearchV1(client, "")
}

// getReposTokenAuthV1 validates the user-specified list of repositories against an index server, e.g., Docker Hub.
// It returns a list of IndexInfo structs with index info for each validated repository.
func getReposTokenAuthV1(repo RepoType, client *http.Client) (indexInfo IndexInfo, e error) {
	_, _, BasicAuth, XRegistryAuth = GetRegistryURL()
	URL := RegistryAPIURL + "/v1/repositories/" + string(repo) + "/images"
	req, e := http.NewRequest("GET", URL, nil)
	req.Header.Set("X-Docker-Token", "true")
	if BasicAuth != "" {
		req.Header.Set("Authorization", "Basic "+BasicAuth)
	}
	r, e := client.Do(req)
	if e != nil {
		except.Error(e, ":getReposTokenAuthV1 HTTP request failed")
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
		var response []byte
		response, e = RegistryQueryV1(client, RegistryAPIURL+"/v1/repositories/"+string(repo)+"/tags")
		if e != nil {
			except.Error(e)
			if s, ok := e.(*HTTPStatusCodeError); ok {
				except.Error("Skipping Repo: %s, tag lookup status code %d", string(repo), s.StatusCode)
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
type V2Schema1FSLayer struct {
	BlobSum string
}
type ManifestV2Schema1 struct {
	SchemaVersion int
	Name          string
	Tag           string
	Architecture  string
	FsLayers      []V2Schema1FSLayer
	History       []V1Compat
	// Ignoring the signatures for now, sorry...
	// Signatures    []string
}

func v2GetMetadata(client *http.Client, repo, tag string) (metadata ImageMetadataInfo, e error) {
	response, err := RegistryQueryV2(client, RegistryAPIURL+"/v2/"+repo+"/manifests/"+tag)
	if err != nil {
		except.Error(err)
		if s, ok := err.(*HTTPStatusCodeError); ok {
			except.Error("Skipping Repo: %s, tag lookup status code %d", string(repo), s.StatusCode)
			e = err
		}
		return
	}

	metadata.Registry = RegistrySpec
	metadata.Repo = repo
	metadata.Tag = tag
	metadata.Image = ""

	var m ManifestV2Schema1
	b := bytes.NewBuffer(response)
	if e = json.NewDecoder(b).Decode(&m); e != nil {
		blog.Warn("Failed to parse manifest")
		return
	}
	if m.SchemaVersion != 1 {
		blog.Warn("Manifest schema version %d is not yet supported\n", m.SchemaVersion)
		e = errors.New("Manifest schema version " + strconv.Itoa(m.SchemaVersion) + " not yet supported by collector")
		return
	}
	if len(m.History) == 0 {
		e = errors.New("repo " + repo + ":" + tag + " no images found in history")
		return
	}
	// Recent versions of Docker daemon (1.8.3+?) have complex code that calculates
	// the image ID from the V2 manifest.
	// This seems to be in flux as Docker moves toward content-addressable images in 1.10+,
	// and as the registry image manifest schema itself is still evolving.
	// As a temporary measure until Docker converges to a more stable state, collector
	// will calculate its own hash over the (re-serialized) V2 manifest and use the calculated
	// value to try to filter out images that have previously been processed.
	// The Docker-calculated image ID will get added to the metadata struct
	// after the image is pulled.
	// blog.Info("Response to /v2/"+repo+"/manifests/"+tag+": %s", string(response))

	serializedManifest, e := json.Marshal(m)
	hash := sha256.Sum256(serializedManifest)
	metadata.ManifestHash = hex.EncodeToString(hash[:])
	blog.Info("Computed manifest hash %s", metadata.ManifestHash)
	// blog.Info("Writing manifest to " + fname)
	// err = ioutil.WriteFile(fname, response, 0644)
	// if err != nil {
	// 	blog.Error(err)
	// }

	var image ImageStruct
	if e = json.Unmarshal([]byte(m.History[0].V1Compatibility), &image); e != nil {
		blog.Warn("Failed to parse ImageStruct")
		return
	}
	var creationTime time.Time
	if creationTime, e = time.Parse(time.RFC3339Nano, image.Created); e != nil {
		blog.Warn("Failed to parse creation time")
		return
	}
	metadata.Datetime = creationTime
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
		except.Error("Unknown registry protocol %s", *RegistryProto)
		return
	}
	panic("Unreachable")
}

func getTagsTokenAuthV1(repo RepoType, client *http.Client, indexInfo IndexInfo) (tagSlice []TagInfo, e error) {
	tagSlice, e = lookupTagsTokenAuthV1(client, indexInfo)
	if e != nil {
		except.Error(e, ": Error in looking up tags in dockerhub")
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
				except.Error("Registry returned HTTP status code %d, skipping %s:%s image %s",
					s.StatusCode, string(repo), string(tag), string(imageID))
				continue
			}
			// some other error (network broken?), so give up
			except.Error(e, "Unable to lookup metadata for",
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

// RegistryRequestWithToken queries a Docker Registry that uses v1 Token Auth, e.g., Docker Hub V1.
func RegistryRequestWithToken(client *http.Client, URL string, dockerToken string) (response []byte, e error) {
	var req *http.Request
	req, e = http.NewRequest("GET", URL, nil)
	if e != nil {
		except.Error(e)
		return
	}
	req.Header.Set("Authorization", "Token "+dockerToken)
	var r *http.Response
	r, e = client.Do(req)
	if e != nil {
		except.Error(e)
		return
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		except.Error(e)
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
		except.Error(e)
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
		except.Error(e, "Unable to query metadata for image: "+string(imageID))
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
func GetNewImageMetadata(oldMetadataSet MetadataSet) (metadataSlice []ImageMetadataInfo, currentMetadataSet MetadataSet) {

	var currentMetadataSlice []ImageMetadataInfo
	//config.BanyanUpdate("Loading Registry Metadata")
	if LocalHost == true {
		blog.Info("Collect images from local Docker host")
		currentMetadataSlice = GetLocalImageMetadata(oldMetadataSet)
	} else {
		currentMetadataSlice = GetImageMetadata(oldMetadataSet)
	}

	// get only the new metadata from currentMetadataSlice
	currentMetadataSet = NewMetadataSet()
	for _, metadata := range currentMetadataSlice {
		currentMetadataSet.Insert(metadata)
		if oldMetadataSet.Exists(metadata) == false {
			// metadata is not in old map
			metadataSlice = append(metadataSlice, metadata)
			blog.Info("New ImageMetadata %+v", metadata)
		}
	}

	// find entries in the old map that are not in the current map,
	// and remove those entries from the database
	obsolete := []ImageMetadataInfo{}
	for metadata := range oldMetadataSet {
		if !currentMetadataSet.Exists(metadata) {
			if len(ReposToProcess) > 0 {
				if _, present := ReposToProcess[RepoType(metadata.Repo)]; present {
					obsolete = append(obsolete, metadata)
					blog.Info("Obsolete ImageMetadata: %v", metadata)
				}
			} else {
				obsolete = append(obsolete, metadata)
				blog.Info("Obsolete ImageMetadata: %+v", metadata)
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
		except.Warn("No image metadata to save!")
		return
	}

	config.BanyanUpdate("Remove Metadata", statusMessageMD(obsolete))

	for _, writer := range WriterList {
		writer.RemoveImageMetadata(obsolete)
	}

	return
}

func v2GetTagsMetadata(repoSlice []RepoType) (metadataSlice []ImageMetadataInfo, e error) {
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
		response, err := RegistryQueryV2(client, RegistryAPIURL+"/v2/"+string(repo)+"/tags/list")
		if err != nil {
			except.Error(err)
			if s, ok := err.(*HTTPStatusCodeError); ok {
				except.Error("Skipping Repo: %s, tag lookup status code %d", string(repo), s.StatusCode)
				continue
			}
			return
		}
		//parse JSON output
		var m V2Tag
		if e = json.Unmarshal(response, &m); e != nil {
			return
		}
		// t := TagInfo{Repo: repo, TagMap: make(map[TagType]ImageIDType)}
		for _, tag := range m.Tags {
			metadata, e := v2GetMetadata(client, string(repo), tag)
			if e != nil {
				except.Error(e, ":Unable to get metadata for repo", string(repo), "tag", tag)
				continue
			}
			// t.TagMap[TagType(tag)] = ImageIDType(metadata.Image)
			metadataSlice = append(metadataSlice, metadata)
		}
		// tagSlice = append(tagSlice, t)
	}
	return
}

// GetImageMetadataSpecified queries the local host, if LocalHost=true, or else a
// Docker V1 registry for info about each image specified in the imageMap argument.
func GetImageMetadataSpecifiedV1(imageMap map[ImageIDType][]RepoTagType,
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
				except.Error(e, "imageID", string(imageID), "not in metadataMap")
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
				response, e = InspectImage(string(imageID))
			} else {
				if *RegistryProto == "quay" {
					// TODO: Properly support quay.io image metadata instead of faking it.
					t := time.Date(2011, time.January, 1, 1, 0, 0, 0, time.UTC)
					metadata.Image = string(imageID)
					metadata.Datetime = t
					ch <- metadata
					return
				}
				response, e = RegistryQueryV1(client, RegistryAPIURL+"/v1/images/"+string(imageID)+"/json")
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
					except.Error(err, ":GetImageMetadataSpecified")
				}
			}
		}
	}
	for ; goCount > 0; goCount-- {
		select {
		case metadata := <-ch:
			metadataSlice = append(metadataSlice, metadata)
		case err := <-errch:
			except.Error(err, ":GetImageMetadataSpecified")
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
			if LocalHost {
				newmd.Registry = string(repotag.Registry)
			} else {
				newmd.Registry = RegistrySpec
			}
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
		except.Warn("No image metadata to save!")
		return
	}

	config.BanyanUpdate("Save Image Metadata", statusMessageMD(metadataSlice))

	slice := []ImageMetadataInfo{}
	for _, metadata := range metadataSlice {
		if len(metadata.Image) > 0 {
			slice = append(slice, metadata)
		}
	}
	if len(slice) == 0 {
		return
	}

	for _, writer := range WriterList {
		writer.AppendImageMetadata(slice)
	}

	return
}

// ValidRepoName verifies that the name of a repo is in a legal format.
// A valid name can optionally include a wildcard "*" but only as the last character.
func ValidRepoName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if len(name) > 256 {
		except.Error("Invalid repo name, too long: %s", name)
		return false
	}
	for i, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
			continue
		case c >= 'A' && c <= 'Z':
			continue
		case c >= '0' && c <= '9':
			continue
		case c == '/' || c == '_' || c == '-' || c == '.':
			continue
		case c == '*' && i == len(name)-1:
			continue
		default:
			except.Error("Invalid repo name %s", name)
			return false
		}
	}
	return true
}
