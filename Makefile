# Copyright 2019 Michael Merrill
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#Test

.PHONY: all
all: all-container

# Use the 0.0 tag for testing, it shouldn't clobber any release builds
TAG ?= latest
REGISTRY ?= mmerrill35
DOCKER ?= docker
SED_I ?= sed -i
GOHOSTOS ?= $(shell go env GOHOSTOS)

ifeq ($(GOHOSTOS),darwin)
  SED_I=sed -i ''
endif

REPO_INFO ?= $(shell git config --get remote.origin.url)
GIT_COMMIT ?= git-$(shell git rev-parse --short HEAD)

PKG = github.com/mmerrill3/wls-ingress

ARCH ?= $(shell go env GOARCH)
GOARCH = ${ARCH}
DUMB_ARCH = ${ARCH}

GOBUILD_FLAGS := -v

ALL_ARCH = amd64 arm64

BUSTED_ARGS =-v --pattern=_test

GOOS = linux

export ARCH
export DUMB_ARCH
export TAG
export PKG
export GOARCH
export GOOS
export GIT_COMMIT
export GOBUILD_FLAGS
export REPO_INFO
export BUSTED_ARGS

IMGNAME = wls-ingress
IMAGE = $(REGISTRY)/$(IMGNAME)
MULTI_ARCH_IMG = $(IMAGE)-$(ARCH)

# Set default base image dynamically for each arch
BASEIMAGE?=alpine:latest

ifeq ($(ARCH),arm64)
	QEMUARCH=aarch64
endif

TEMP_DIR := $(shell mktemp -d)

DOCKERFILE := $(TEMP_DIR)/Dockerfile

.PHONY: sub-container-%
sub-container-%:
	$(MAKE) ARCH=$* build container

.PHONY: sub-push-%
sub-push-%:
	$(MAKE) ARCH=$* push

.PHONY: all-container
all-container: $(addprefix sub-container-,$(ALL_ARCH))

.PHONY: all-push
all-push: $(addprefix sub-push-,$(ALL_ARCH))

.PHONY: container
container: clean-container .container-$(ARCH)

.PHONY: .container-$(ARCH)
.container-$(ARCH):
	mkdir -p $(TEMP_DIR)/rootfs
	cp bin/$(ARCH)/wls-ingress-controller $(TEMP_DIR)/rootfs/wls-ingress-controller
	cp Dockerfile $(TEMP_DIR)/Dockerfile
	$(SED_I) "s|BASEIMAGE|$(BASEIMAGE)|g" $(DOCKERFILE)
	@$(DOCKER) build --no-cache --pull -t $(IMAGE):$(TAG) $(TEMP_DIR)

.PHONY: clean-container
clean-container:
	@$(DOCKER) rmi -f $(IMAGE):$(TAG) || true

.PHONY: push
push: .push-$(ARCH)

.PHONY: .push-$(ARCH)
.push-$(ARCH):
	$(DOCKER) push $(IMAGE):$(TAG)

.PHONY: build
build:
	@build/build.sh

.PHONY: clean
clean:
	rm -rf bin/ .gocache/

.PHONY: static-check
static-check:
	@build/static-check.sh

.PHONY: test
test:
	@build/test.sh

.PHONY: vet
vet:
	@go vet $(shell go list ${PKG}/internal/... | grep -v vendor)

.PHONY: release
release: all-container all-push
	echo "done"

.PHONY: dep-ensure
dep-ensure:
	dep ensure -v
