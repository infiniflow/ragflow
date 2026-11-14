PREFIX?=docker.io
RAGFLOW_VERSION?=$(shell git describe --tags --match=v* --first-parent --always)
TAG:=$(RAGFLOW_VERSION)
GIT_COMMIT=$(shell git rev-parse --short HEAD)
TIMETAG:=$(shell date +'%Y%m%d%H%M')

# docker or podman
docker=podman
dockerbuildflags= --format docker
frontend:
	@echo "$@: $(TAG)"
	@${docker} build ${dockerbuildflags} -f Dockerfile.frontend -t $(PREFIX)/ragflow-fe:$(TAG) \
		--build-arg ADMIN_UPSTREAM=api.ragflow.svc \
		--build-arg API_UPSTREAM=api.ragflow.svc \
		--build-arg RAGFLOW_VERSION=${RAGFLOW_VERSION} \
		.
	@${docker} tag $(PREFIX)/ragflow-fe:$(TAG) ragflow-fe:latest
	@echo "push image command: ${docker} push $(PREFIX)/ragflow-fe:$(TAG)"


ragflow:
	@echo "$@: $(TAG)"
	@${docker} build ${dockerbuildflags} -f Dockerfile -t $(PREFIX)/ragflow:$(TAG) \
		--build-arg RAGFLOW_VERSION=${RAGFLOW_VERSION} \
		--build-arg NEED_MIRROR=1 \
		--build-arg GITEE_TOKEN= \
		.
	@${docker} tag $(PREFIX)/ragflow:$(TAG) ragflow:latest
	@echo "push image command: ${docker} push $(PREFIX)/ragflow:$(TAG)"

all: frontend ragflow
.PHONY: all
