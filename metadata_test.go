package collector

import (
	"fmt"
	"net/http"
	"testing"

	config "github.com/banyanops/collector/config"
)

func TestGetLocalImageMetadata(t *testing.T) {
	fmt.Println("TestGetLocalImageMetadata")

	var e error
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = config.DockerHub
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		OtherMetadata: OtherMetadata{
			Repo: "fedora",
			Tag:  "latest",
		},
	}
	fmt.Println("TestPullImage %v", metadata)
	PullImage(&metadata)

	var currentMetadataSlice []ImageMetadataInfo
	MetadataSet := NewMetadataSet()
	LocalHost = true
	ReposToProcess[RepoType(metadata.Repo)] = true
	currentMetadataSlice = GetLocalImageMetadata(MetadataSet)

	for _, localImage := range currentMetadataSlice {
		fmt.Println("localImage: ", localImage)
		if localImage.Repo == metadata.Repo && localImage.Tag == metadata.Tag {
			fmt.Println("TestGetLocalImageMetadata succeeded for ", metadata)
			return
		}
	}
	t.Fatal("TestGetLocalImageMetadata failed for ", metadata)
	return
}

func TestValidRepoName(t *testing.T) {
	testCases := map[string]bool{
		"library/ubuntu": true,
		"abc/def/ghi":    true,
		"q:x&2":          false,
		"banyan/*":       true,
		"foo*bar/xyz":    false,
	}
	for repo, expected := range testCases {
		if ValidRepoName(repo) != expected {
			t.Fatalf("TestValidRepoName ValidRepoName(%s) did not return %v", repo, expected)
		}
	}
}

func TestRegistryQuery(t *testing.T) {
	var e error
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = config.DockerHub
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	fmt.Printf("RegistryAPIURL %s HubAPI %v BasicAuth %s XRegistryAuth %s\n", RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth)
	*RegistryProto = "v2"
	client := &http.Client{}
	r, e := RegistryQueryV2(client, "https://"+RegistrySpec+"/v2/banyanops/collector/tags/list")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println(string(r))
}
