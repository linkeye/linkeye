FROM ubuntu:xenial

ENV PATH=/usr/lib/go-1.9/bin:$PATH

RUN \
  apt-get update && apt-get upgrade -q -y && \
  apt-get install -y --no-install-recommends golang-1.9 git make gcc libc-dev ca-certificates && \
  git clone --depth 1 --branch release/1.0.0 https://github.com/linkeye/linkeye && \
  (cd linkeye && make linkeye) && \
  cp linkeye/build/bin/linkeye /linkeye && \
  apt-get remove -y golang-1.9 git make gcc libc-dev && apt autoremove -y && apt-get clean && \
  rm -rf /linkeye

EXPOSE 8545
EXPOSE 38883

ENTRYPOINT ["/linkeye"]
