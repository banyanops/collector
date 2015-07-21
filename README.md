# Banyan Collector: A framework to *peek* inside containers

Have you wondered what your container images really contain? If they have the very packages that are susceptible to all kinds of attacks? Or, if they have the configuration you expect when they are run? Banyan Collector provides a powerful, extensible framework to answer all these questions and more.

Banyan Collector is a light-weight, easy to use, and modular system that allows you to launch containers from a registry, run arbitrary scripts inside them, and gather useful information. This framework can be used to statically analyze images for several purposes including:
* Collect specific information from all images (e.g., packages installed)
* Enforce policies (e.g., no unauthorized user accounts, etc.)
* Validate invariants (e.g., nginx.conf is present in the right directory, etc.)
* and so on...

## Getting started

The collector can be run in one of two modes: (a) as a standalone executable (b) in a container. The main requirement is to run the collector on a Docker Host (machine that has the Docker Daemon running). If you want to collect data from a private registry make sure you are logged into it (sudo docker login REGISTRY)

(a) To run it as a standalone executable, you need *go* in your environment (https://golang.org/doc/install). Once *go* is installed, just run the following on a Docker Host:

    $ go get -u github.com/banyanops/collector/...
    $ cd <COLLECTOR_SOURCE_DIR>; sudo COLLECTOR_DIR=$PWD $GOPATH/bin/collector <REGISTRY> <REPO>

where REGISTRY is either a private registry (e.g., http://reg.myorg.com) or Docker Hub (index.docker.io), and REPO is a repository for which you'd like to collect data. For a private registry (with search enabled), if no REPO is specified, data is collected from all the repositories. Collector also supports the Google Container Registry (*.gcr.io), quay.io, and collecting from local images instead of pulling from a registry (by specifying "local.host" as the REGISTRY).

(b) To run the collector in a container, please follow instructions on [Docker Hub](https://registry.hub.docker.com/u/banyanops/collector/).

More generally, collector can be configured using several options (e.g., registry poll interval, remove images threshold, secure registry settings, etc.): 

    $ sudo collector [options] REGISTRY [REPO1 REPO2 ...]

For a list of all the options run:

    $ collector -h

## Why not just use shell scripts?

Shell scripts are great for quickly getting specific information from images. However, as we add more complexity, it becomes hard to write scripts that are easy to maintain, quickly extensible and portable to different environments.

For example, the complexity in managing registry connections, keeping track of repo/tag changes, cleaning up stale images, supporting arbitrary policies, etc. is too much to handle using simple scripts. Furthermore, packaging the collector in a container provides a portable framework that is not dependent on any particular host configuration, and can seamlessly run anywhere.

## Tests
    
The go tests rely on write access to the Docker UNIX socket /var/run/docker.sock. One approach is to add the user to the "docker" group to enable write access to the socket. Alternatively, the tests can be run as "root", for example using "sudo", but this requires the root user to share the Go development environment ($GOPATH, etc.).

Another requirement to run the tests is to set environment variables $DOCKER_USER and $DOCKER_PASSWORD to the user's Docker Hub login credentials. Additionally, the tests emit a warning if $COLLECTOR_DIR is not set (but the warning can be safely ignored).

Once the environment has been correctly setup, go tests can be run using the standard command:

    $ go test

## More information

More details about Collector operation/architecture, etc. are available under [docs](/docs).

For further details about how one might use this in an enterprise, please check out [Banyan](http://www.banyanops.com). This SAAS service offers deeper analysis of your data and provides a dashboard showing which of your images are compliant to your policies (e.g., which of your images have security vulnerabilities, etc.) along with real-time updates and email notifications. 

## Get involved

Collector is under active development. Fork the project and submit pull requests, or file issues or tweet us [@banyanops](https://twitter.com/banyanops).

## License

Banyan Collector is distributed under Apache 2.0 License. More details in [LICENSE](/LICENSE).
