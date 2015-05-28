package collector

import (
	"fmt"
	"os"
	"testing"
)

var imi []ImageMetadataInfo
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

func TestPullImage(t *testing.T) {
	fmt.Println("TestPullImage")
	var e error
	DockerTransport, e = NewDockerTransport(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = "index.docker.io"
	RegistryAPIURL, HubAPI, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		Repo: "busybox",
		Tag:  "latest",
	}
	fmt.Println("TestPullImage %v", metadata)
	PullImage(metadata)
	return
}

func TestRemoveImage(t *testing.T) {
	fmt.Println("TestRemoveImage")
	TestPullImage(t)
	metadata1 := ImageMetadataInfo{
		Repo: "busybox",
		Tag:  "latest",
	}
	metadata2 := ImageMetadataInfo{
		Repo: "busybox",
		Tag:  "buildroot-2014.02",
	}
	fmt.Println("TestRemoveImage %v %v", metadata1, metadata2)
	RemoveImages([]ImageMetadataInfo{metadata1}, GetImageToMDMap([]ImageMetadataInfo{metadata1, metadata2}))
	return
}

func dockerAuth() (user, password, registry string, e error) {
	user = os.Getenv("DOCKER_USER")
	password = os.Getenv("DOCKER_PASSWORD")
	registry = os.Getenv("DOCKER_REGISTRY")
	if registry == "" {
		registry = "index.docker.io"
	}
	RegistryAPIURL = "https://" + user + ":" + password + "@" + registry

	if user == "" || password == "" {
		e = fmt.Errorf("Please put valid credentials for registry " + registry + " in envvars DOCKER_USER and DOCKER_PASSWORD.")
		return
	}
	return
}

func TestGetReposHub(t *testing.T) {
	fmt.Println("TestGetReposHub")
	_, _, registry, e := dockerAuth()
	if e != nil {
		t.Fatal(e)
	}
	if registry != "index.docker.io" {
		t.Fatal("TestRegReposHub only works with DOCKER_REGISTRY=index.docker.io")
	}
	ReposToProcess["library/mysql"] = true
	//reposToProcess["ncarlier/redis"] = true
	hubInfo, e := getReposHub()
	if e != nil {
		t.Fatal(e)
	}
	if hubInfo == nil || len(hubInfo) == 0 {
		t.Fatal("hubInfo is nil")
	}
	fmt.Print(hubInfo, e)
	return
}

func TestGetTagsMetadataHub(t *testing.T) {
	fmt.Println("TestGetTagsMetadataHub")
	_, _, registry, e := dockerAuth()
	if e != nil {
		t.Fatal(e)
	}
	if registry != "index.docker.io" {
		t.Fatal("TestGetTagsMetadataHub only works with DOCKER_REGISTRY=index.docker.io")
	}
	ReposToProcess["library/mysql"] = true
	hubInfo, e := getReposHub()
	if e != nil {
		t.Fatal(e)
	}
	if hubInfo == nil || len(hubInfo) == 0 {
		t.Fatal("hubInfo is nil")
	}
	oldImiSet := NewImiSet()
	tagSlice, imi, e := getTagsMetadataHub(hubInfo, oldImiSet)
	if e != nil {
		t.Fatal(e)
	}
	if tagSlice == nil || len(tagSlice) == 0 {
		t.Fatal("tagSlice", tagSlice)
	}
	if imi == nil || len(imi) == 0 {
		t.Fatal("imi", imi)
	}
	fmt.Print(tagSlice)
	return
}

func TestParseDistro(t *testing.T) {
	fmt.Println("TestParseDistro")
	var tests = []struct {
		pretty   string
		codename string
	}{
		{"Ubuntu 14.04.1 LTS", "UBUNTU-trusty"},
		{"CentOS Linux 7 (Core)", "REDHAT-7Server"},
	}
	for _, trial := range tests {
		distro := getDistroID(trial.pretty)
		if distro != trial.codename {
			t.Fatal("input:", trial.pretty, "output", distro, "expected:", trial.codename)
		}
		fmt.Println("Found distro: ", distro)
	}
	return
}
