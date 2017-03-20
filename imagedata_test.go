package collector

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"

	config "github.com/banyanops/collector/config"
)

var metadataSlice []ImageMetadataInfo
var tagSlice []TagInfo

func TestMain(m *testing.M) {
	fmt.Println("TestMain: Run First")
	// make sure environment vars have been setup
	_, _, _, e := dockerAuth()
	if e != nil {
		fmt.Println(e)
		os.Exit(55)
	}
	os.Exit(m.Run())
}

func TestPullImageOne(t *testing.T) {
	fmt.Println("TestPullImage")
	var e error
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = config.DockerHub
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		OtherMetadata: OtherMetadata{
			Repo: "library/busybox",
			Tag:  "latest",
		},
	}
	fmt.Println("TestPullImage %v", metadata)
	err := PullImage(&metadata)
	fmt.Printf("final metadata is %#v\n", metadata)
	if err != nil {
		t.Fatal(e)
	}
	return
}

func TestPullImageBogusID(t *testing.T) {
	fmt.Println("TestPullImageBogusID")
	var e error
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = config.DockerHub
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		Image: "Bogus",
		OtherMetadata: OtherMetadata{
			Repo: "busybox",
			Tag:  "latest",
		},
	}
	fmt.Println("TestPullImage %v", metadata)
	err := PullImage(&metadata)
	if err == nil {
		t.Fatal("PullImage was supposed to return an error here")
	}
	if !strings.HasPrefix(err.Error(), "PullImage busybox") {
		t.Fatal("Unexpected error: " + err.Error())
	}
	fmt.Printf("Received expected error %s\n", err.Error())
	return
}

func TestRemoveImage(t *testing.T) {
	fmt.Println("TestRemoveImage")
	TestPullImageOne(t)
	metadata1 := ImageMetadataInfo{
		OtherMetadata: OtherMetadata{
			Repo: "library/busybox",
			Tag:  "latest",
		},
	}
	/*
		metadata2 := ImageMetadataInfo{
			Repo: "busybox",
			Tag:  "buildroot-2014.02",
		}
	*/
	// fmt.Println("TestRemoveImage %v %v", metadata1, metadata2)
	fmt.Println("TestRemoveImage %v", metadata1)
	RemoveImages([]ImageMetadataInfo{metadata1})
	return
}

func dockerAuth() (user, password, registry string, e error) {
	user = os.Getenv("DOCKER_USER")
	password = os.Getenv("DOCKER_PASSWORD")
	registry = os.Getenv("DOCKER_REGISTRY")
	if registry == "" {
		registry = config.DockerHub
	}
	RegistryAPIURL = "https://" + registry
	s := user + ":" + password
	BasicAuth = base64.StdEncoding.EncodeToString([]byte(s))

	if user == "" || password == "" {
		e = fmt.Errorf("Please put valid credentials for registry " + registry + " in envvars DOCKER_USER and DOCKER_PASSWORD.")
		return
	}
	return
}

/* Obsolete V1 token auth test.
func TestGetReposHub(t *testing.T) {
	fmt.Println("TestGetReposHub")
	_, _, registry, e := dockerAuth()
	if e != nil {
		t.Fatal(e)
	}
	if registry != config.DockerHub {
		t.Fatal("TestRegReposHub only works with DOCKER_REGISTRY=" + config.DockerHub)
	}
	ReposToProcess["library/mysql"] = true
	//reposToProcess["ncarlier/redis"] = true
	repo := RepoType("library/mysql")
	client := &http.Client{}
	indexInfo, e := getReposTokenAuthV1(repo, client)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Print(indexInfo, e)
	return
}
*/

func TestGetTagsMetadataHub(t *testing.T) {
	var e error
	fmt.Println("TestGetTagsMetadataHub")
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = config.DockerHub
	_, _, registry, e := dockerAuth()
	if e != nil {
		t.Fatal(e)
	}
	if registry != config.DockerHub {
		t.Fatal("TestGetTagsMetadataHub only works with DOCKER_REGISTRY=" + config.DockerHub)
	}
	ReposToProcess["library/iojs"] = true
	repo := RepoType("library/iojs")
	repoSlice := []RepoType{repo}
	metadataSlice, e = v2GetTagsMetadata(repoSlice)
	if metadataSlice == nil || len(metadataSlice) == 0 {
		t.Fatal("metadataSlice", metadataSlice)
	}
	fmt.Printf("metadataSlice len=%d, contents %v\n", len(metadataSlice), metadataSlice)
	return
}
