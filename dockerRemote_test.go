package collector

import (
	"fmt"
	"testing"

	config "github.com/banyanops/collector/config"
)

func TestListDanglingImages(t *testing.T) {
	fmt.Println("ListDanglingImages")
	var err error
	DockerClient, err = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if err != nil {
		t.Fatal(err)
	}
	imageList, err := ListDanglingImages()
	if err != nil {
		t.Fatal(err)
	}
	for _, image := range imageList {
		fmt.Printf("image ID %s\n", string(image))
	}
}

func TestRemoveImageByID(t *testing.T) {
	var err error
	DockerClient, err = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if err != nil {
		t.Fatal(err)
	}
	RegistrySpec = config.DockerHub
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		OtherMetadata: OtherMetadata{
			Repo: "banyanops/nginx",
			Tag:  "1.7",
		},
	}
	fmt.Println("TestPullImage %v", metadata)
	PullImage(&metadata)

	id := "d052f9300189"
	resp, err := RemoveImageByID(ImageIDType(id))
	if err != nil {
		id = "bb65d19fc17c"
		resp, err = RemoveImageByID(ImageIDType(id))
		if err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println(string(resp))
}
