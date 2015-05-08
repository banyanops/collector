# Banyan Collector: A framework to *peek* inside containers

Have you wondered what your container images really contain? If they have the very packages that are susceptible to all kinds of attacks? Or, has the configuration you expect when they are actually running? Banyan Collector provides a powerful, extensible framework to answer all these questions and more...

Banyan Collector is a light-weight, easy to use, and modular system that allows you to launch containers from a registry, run arbitrary scripts inside them, and gather useful information. This framework can be used to statically analyze images for several purposes including:
* Collect specific information from all images (e.g., packages installed)
* Enforce policies (e.g., no unauthorized user accounts, etc.)
* Validate invariants (e.g., nginx.conf is present in the right directory, etc.)
* and so on...

## Install

## Requirements

## Getting started

The collector can be run in one of two modes. (a) as a standalone executable (b) in a container. 

(a) To run it as a standalone executable, just run the following on a Docker host (machine running docker daemon):

    $ sudo docker login REGISTRY
    $ go get -u banyan/collector
    $ sudo collector -localvolume=true -outdest=file REGISTRY
 
where REGISTRY is either a private registry (e.g., http://reg.myorg.com) or Docker Hub (index.docker.io). More generally, collector can be configured using several options (e.g., registry poll interval, remove images, secure registry, etc.): 

    $ collector [options] REGISTRY [Repo...] 

For a list of all the options run:

    $ collector -h

(b) To run the collector in a container:

    $ docker run -d \
    -v $(which docker):/usr/bin/docker \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v $HOME/.dockercfg:/root/.dockercfg \
    banyanops/collector {{REGISTRY}}

## More information

Documents, banyanops.com, UI, etc.

## License

Banyan Collector is distributed under Apache 2.0 License. More details in [LICENSE](/LICENSE).
