RUNTIME 					 = $(shell which podman 2>/dev/null || which docker)
EFFECTIVE_VERSION := $(shell git rev-parse HEAD | cut -c1-8)

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test ./... -v

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"'

.PHONY: minikube-deploy
minikube-deploy:
	# @if ! minikube addons list | grep -E '\sregistry\s' | grep -q enabled; then\
	# 		echo "enabling minikube registry";\
	# 		minikube addons enable registry;\
	# fi
	$(RUNTIME) build -t $(shell minikube ip):5000/podchaosmonkey:$(EFFECTIVE_VERSION) .
	$(RUNTIME) push --tls-verify=false $(shell minikube ip):5000/podchaosmonkey:$(EFFECTIVE_VERSION)
	cd kustomize/podchaosmonkey && kustomize edit set image podchaosmonkey=$(shell minikube ip):5000/podchaosmonkey:$(EFFECTIVE_VERSION)
	kustomize build kustomize/ | kubectl apply -f -
