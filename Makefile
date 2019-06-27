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

.PHONY: all
all: all-container

# Use the 0.0 tag for testing, it shouldn't clobber any release builds
TAG ?= 0.0.1
REGISTRY ?= mmerrill3/wls-ingress
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

IMGNAME = wls-ingress-controller
IMAGE = $(REGISTRY)/$(IMGNAME)
MULTI_ARCH_IMG = $(IMAGE)-$(ARCH)

# Set default base image dynamically for each arch
BASEIMAGE?=alpine:latest

ifeq ($(ARCH),arm64)
	QEMUARCH=aarch64
endif

TEMP_DIR := $(shell mktemp -d)

DOCKERFILE := $(TEMP_DIR)/rootfs/Dockerfile

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
	cp bin/$(ARCH)/dbg $(TEMP_DIR)/rootfs/dbg

	cp -RP ./* $(TEMP_DIR)
	$(SED_I) "s|BASEIMAGE|$(BASEIMAGE)|g" $(DOCKERFILE)
	$(SED_I) "s|DUMB_ARCH|$(DUMB_ARCH)|g" $(DOCKERFILE)

ifeq ($(ARCH),amd64)
	# When building "normally" for amd64, remove the whole line, it has no part in the amd64 image
	$(SED_I) "/CROSS_BUILD_/d" $(DOCKERFILE)
else
	# When cross-building, only the placeholder "CROSS_BUILD_" should be removed
	curl -sSL https://github.com/multiarch/qemu-user-static/releases/download/$(QEMUVERSION)/x86_64_qemu-$(QEMUARCH)-static.tar.gz | tar -xz -C $(TEMP_DIR)/rootfs
	$(SED_I) "s/CROSS_BUILD_//g" $(DOCKERFILE)
endif

	@$(DOCKER) build --no-cache --pull -t $(MULTI_ARCH_IMG):$(TAG) $(TEMP_DIR)/rootfs

ifeq ($(ARCH), amd64)
	# This is for maintaining backward compatibility
	@$(DOCKER) tag $(MULTI_ARCH_IMG):$(TAG) $(IMAGE):$(TAG)
endif

.PHONY: clean-container
clean-container:
	@$(DOCKER) rmi -f $(MULTI_ARCH_IMG):$(TAG) || true

.PHONY: push
push: .push-$(ARCH)

.PHONY: .push-$(ARCH)
.push-$(ARCH):
	$(DOCKER) push $(MULTI_ARCH_IMG):$(TAG)
ifeq ($(ARCH), amd64)
	$(DOCKER) push $(IMAGE):$(TAG)
endif

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
