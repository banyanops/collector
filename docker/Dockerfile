FROM ubuntu:14.04

MAINTAINER yoshio@banyanops.com

RUN apt-get update && apt-get install -y ca-certificates

ENV COLLECTOR_DIR /banyancollector
ENV BANYAN_DIR /banyandir
ENV PATH $COLLECTOR_DIR:$PATH
WORKDIR $COLLECTOR_DIR
COPY collector git_info.txt $COLLECTOR_DIR/
COPY data/bin $COLLECTOR_DIR/data/bin
COPY data/defaultscripts $COLLECTOR_DIR/data/defaultscripts

ENTRYPOINT ["/banyancollector/collector"]
