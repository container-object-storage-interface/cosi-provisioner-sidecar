# 
# Copyright 2020 The Kubernetes Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# 
PATH_TO_MAIN=./cmd
GIT_SHA := $(shell git describe --always --abbrev=7 --match "NOT A TAG" --dirty="-dirty" || echo "unknown")
export GIT_SHA
GO_BLD_ARCH  ?= $(shell go env GOHOSTARCH)
GO_BLD_OS ?=$(shell go env GOOS)
BINARY_PATH=bin/sample-provisioner_${GO_BLD_OS}_${GO_BLD_ARCH}


GO_BUILD_CMD := CGO_ENABLED=0 GOOS=$(GO_BLD_OS) GOARCH=$(GO_BLD_ARCH) \
    go build -ldflags="\
    -X 'github.com/container-object-storage-interface/cosi-provisioner-sidecar/examples/sample-provisioner/driver.VERSION=$(GIT_SHA)'"

all:	build
build: 
	@echo "building sample-provisioner..." 
	@$(GO_BUILD_CMD)  -o $(BINARY_PATH) $(PATH_TO_MAIN) 
    
clean: 
	@echo "cleaning the environment..."
	rm -f $(BINARY_PATH)

run:
	@echo "starting grpc server..."
	./bin/sample-provisioner_${GO_BLD_OS}_${GO_BLD_ARCH}  \
	  --listen-address tcp://0.0.0.0:9000 \
	  --s3-endpoint localhost:9001 \
	  --access-key AKIAIOSFODNN7EXAMPLE \
	  --secret-key wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY 