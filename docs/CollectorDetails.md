
In this document we present some of the key aspects of collector implementation.

## Collector Operation

![Alt text](/resources/CollectorOperation.png?raw=true "Collector Operation")

The figure above shows the overall operation of the collector. Step (0) involves Collector talking to registry (private or docker hub’s index) to obtain image hashes and repo/tags. This step is optional — a user could just specify the repositories of interest and the collector only collects data for these repos.

Here are the rest of the steps:

1. Collector talks to the Docker Daemon on it’s local Docker Host and places a docker *pull* request for repo/tags of interest
2. Docker daemon passes on the *pull* request to docker registry which brings all the layers comprising the image into the Docker Host
3. Collector now issues a docker *run* request for the containers to be inspected. We currently only support running one container at a time to ensure that our system doesn’t use up too many resources, but we can easily extend it to run multiple of them simultaneously. The request also contains volumes to mount from the collector container that contains statically-linked tools (e.g., bash-static, python-static, etc.) and the directories where scripts are located. We also specify a special entrypoint so that we override any CMDs/entrypoint from the original container.
4. Docker Daemon launches the containers to be inspected. All containers run a script (e.g., banyan or user-specified script) and produces output on stdout.
5. The containers output is collated by the collector.
6. The final output can be sent to Banyan Analyzer for further analysis, or just stored in the local file-system against which additional scripts can be run. 

Steps 1-6 are repeated for every script. Note that all the scripts could have been executed in tandem once a container is launched. We decided against this because we wanted each script to run starting from a clean slate (e.g., didn’t want one script to affect another).

## Collector Architecture

The collector has been designed so that it is modular and extensible with different plugins. 

![Alt text](/resources/CollectorArchitecture.png?raw=true "Collector Architecture")

The figure above shows the overall collector architecture. At the center is the collector core that takes in inputs from various plugins, and then launches/collects data for desired containers (as described in the previous section). Here are some of the plugins where we encourage users to contribute/submit pull requests:
* Registry: We currently support both private registry and DockerHub as the source of image location. But a given collector instance can only run on a single registry (one private registry or docker hub). However, you can run multiple instances of the collector pointing to different private registries and/or docker hub.
  * Possible extensions: multiple registry support, images in the local filesystem (e.g., not uploaded to registry)
* User-specified scripts: We support multiple types of plugins to write scripts for data collection including Bash and Python. We provide statically linked versions of bash and python, and busybox commands by exploring volumes into the containers to be inspected. That way, we don’t rely on any pre-existing tools inside the container to run scripts. We’ve also provided two sample bash scripts: PkgExtract and PkgDeps that collect package information and dependencies between different packages.
  * Possible extensions: Ruby, Go itself, etc.
* Writer plugin: The Writer interface supports multiple backend writers for the data that is collected by running the scripts inside the containers. We currently have backend implementations for writing output to a file, or sending it to Banyan service for further analysis. 
  * Possible extensions: Socket, localDB, etc.
* User-specified tunables: Several parameters can be set based on your specific requirements. Some examples are: polling interval to the registry to track any updates, repositories of interest, the order in which images are pulled or removed (currently we do it in time order - newest to oldest), number of containers to be launched simultaneously (currently we only support one, as described in the previous section).
  * Possible extensions: Custom algorithms for image pull/rm order, increase concurrency, etc.
