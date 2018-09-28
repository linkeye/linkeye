# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: linkeye android ios linkeye-cross swarm evm all test clean
.PHONY: linkeye-linux linkeye-linux-386 linkeye-linux-amd64 linkeye-linux-mips64 linkeye-linux-mips64le
.PHONY: linkeye-linux-arm linkeye-linux-arm-5 linkeye-linux-arm-6 linkeye-linux-arm-7 linkeye-linux-arm64
.PHONY: linkeye-darwin linkeye-darwin-386 linkeye-darwin-amd64
.PHONY: linkeye-windows linkeye-windows-386 linkeye-windows-amd64

GOBIN = $(shell pwd)/build/bin
GO ?= latest

linkeye:
	build/env.sh go run build/ci.go install ./cmd/linkeye
	@echo "Done building."
	@echo "Run \"$(GOBIN)/linkeye\" to launch linkeye."

swarm:
	build/env.sh go run build/ci.go install ./cmd/swarm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/swarm\" to launch swarm."

all:
	build/env.sh go run build/ci.go install

android:
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/linkeye.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/Geth.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

lint: ## Run linters.
	build/env.sh go run build/ci.go lint

clean:
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/kevinburke/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go get -u github.com/golang/protobuf/protoc-gen-go
	env GOBIN= go install ./cmd/abigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

# Cross Compilation Targets (xgo)

linkeye-cross: linkeye-linux linkeye-darwin linkeye-windows linkeye-android linkeye-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-*

linkeye-linux: linkeye-linux-386 linkeye-linux-amd64 linkeye-linux-arm linkeye-linux-mips64 linkeye-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-*

linkeye-linux-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/linkeye
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep 386

linkeye-linux-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/linkeye
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep amd64

linkeye-linux-arm: linkeye-linux-arm-5 linkeye-linux-arm-6 linkeye-linux-arm-7 linkeye-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep arm

linkeye-linux-arm-5:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/linkeye
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep arm-5

linkeye-linux-arm-6:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/linkeye
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep arm-6

linkeye-linux-arm-7:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/linkeye
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep arm-7

linkeye-linux-arm64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/linkeye
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep arm64

linkeye-linux-mips:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/linkeye
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep mips

linkeye-linux-mipsle:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/linkeye
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep mipsle

linkeye-linux-mips64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/linkeye
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep mips64

linkeye-linux-mips64le:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/linkeye
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-linux-* | grep mips64le

linkeye-darwin: linkeye-darwin-386 linkeye-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-darwin-*

linkeye-darwin-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/linkeye
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-darwin-* | grep 386

linkeye-darwin-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/linkeye
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-darwin-* | grep amd64

linkeye-windows: linkeye-windows-386 linkeye-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-windows-*

linkeye-windows-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/linkeye
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-windows-* | grep 386

linkeye-windows-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/linkeye
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/linkeye-windows-* | grep amd64
