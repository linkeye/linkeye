FROM alpine:3.7

RUN \
  apk add --update go git make gcc musl-dev linux-headers ca-certificates && \
  git clone --depth 1 https://github.com/linkeye/linkeye && \
  (cd linkeye && make linkeye) && \
  cp linkeye/build/bin/linkeye /linkeye && \
  apk del go git make gcc musl-dev linux-headers && \
  rm -rf /linkeye && rm -rf /var/cache/apk/*

EXPOSE 8545
EXPOSE 38883

ENTRYPOINT ["/linkeye"]
