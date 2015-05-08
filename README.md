# Banyan Collector: A framework to *peek* inside containers

Banyan Collector is a light-weight, easy to use, and modular system that allows you to launch containers from a registry, run arbitrary scripts inside them, and gather useful information. This framework can be used to statically analyze images for several purposes including:
* Collect specific information from all images (e.g., packages installed)
* Enforce policies (e.g., no unauthorized user accounts, etc.)
* Validate invariants (e.g., nginx.conf is present in the right directory, etc.)
* and so on...

## Getting started

The collector can be run in one of two modes. (a) in a container (b) as a standalone executable. The main requirement is to run the collector on a Docker Host (machine that has the Docker Daemon running).

(a) To run the collector in a container:

    $ sudo docker login REGISTRY
    $ sudo docker pull banyanops/collector
    $ sudo docker run -d \
    -v $(which docker):/usr/bin/docker \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v $HOME/.dockercfg:/root/.dockercfg \
    banyanops/collector REGISTRY
    
where REGISTRY is either a private registry (e.g., http://reg.myorg.com) or Docker Hub (index.docker.io). 

(b) To run it as a standalone executable, you need *go* in your environment (https://golang.org/doc/install). Once *go* is installed, just run the following on a Docker Host:

    $ sudo docker login REGISTRY
    $ go get -u github.com/banyanops/collector
    $ sudo collector -localvolume=true -outdest=file REGISTRY
 
More generally, collector can be configured using several options (e.g., registry poll interval, remove images threshold, secure registry settings, etc.): 

    $ sudo docker run ... banyanops/collector [options] REGISTRY [Repo...]
    $ sudo collector [options] REGISTRY [Repo...]

For a list of all the options run:

    $ collector -h

## More information

More details about Collector operation/architecture, etc. are availble under [docs](/docs/CollectorDetails.md).

For further details about how one might use this in an enterprise please checkout www.banyanops.com. This SAAS service offers deeper analysis of your data and provides a dashboard showing which of your images are compliant to your policies (e.g., which of your images have security vulnerabilities, etc.) along with real-time updates and email notifications. 

## License

Banyan Collector is distributed under Apache 2.0 License. More details in [LICENSE](/LICENSE).
