FROM centurylink/ca-certs

MAINTAINER yoshio@banyanops.com

ENV COLLECTOR_DIR /banyancollector
ENV HOME /root
ENV BANYAN_DIR /banyandir
ENV PATH $COLLECTOR_DIR:$PATH
WORKDIR $COLLECTOR_DIR
COPY data/bin $COLLECTOR_DIR/data/bin
COPY data/defaultscripts $COLLECTOR_DIR/data/defaultscripts
RUN ["data/bin/busybox", "ln", "-s", "data/bin/busybox", "cp"]
COPY collector git_info.txt $COLLECTOR_DIR/

ENTRYPOINT ["/banyancollector/collector"]
