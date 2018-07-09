FROM alpine:3.7

RUN \
  apk add --update go git make gcc musl-dev linux-headers ca-certificates && \
  git clone --depth 1 https://github.com/linkeye/linkeye && \
  (cd linkeye && make geth) && \
  cp linkeye/build/bin/geth /geth && \
  apk del go git make gcc musl-dev linux-headers && \
  rm -rf /linkeye && rm -rf /var/cache/apk/*

EXPOSE 8545
EXPOSE 30303

ENTRYPOINT ["/geth"]
