package collector

import (
	"fmt"
	"testing"
)

func TestGetLocalImageMetadata(t *testing.T) {
	fmt.Println("TestGetLocalImageMetadata")

	var e error
	DockerTransport, e = NewDockerTransport(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = "index.docker.io"
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		Repo: "fedora",
		Tag:  "latest",
	}
	fmt.Println("TestPullImage %v", metadata)
	PullImage(metadata)

	var currentMetadataSlice []ImageMetadataInfo
	MetadataSet := NewMetadataSet()
	LocalHost = true
	ReposToProcess[RepoType(metadata.Repo)] = true
	currentMetadataSlice = GetLocalImageMetadata(MetadataSet)

	for _, localImage := range(currentMetadataSlice) {
		fmt.Println("localImage: ", localImage)
		if localImage.Repo == metadata.Repo && localImage.Tag == metadata.Tag {
			fmt.Println("TestGetLocalImageMetadata succeeded for ", metadata)
			return
		}
	}
	t.Fatal("TestGetLocalImageMetadata failed for ", metadata)
	return
}
