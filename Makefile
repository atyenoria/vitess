# Copyright 2012, Google Inc. All rights reserved.
# Use of this source code is governed by a BSD-style license that can
# be found in the LICENSE file.

MAKEFLAGS = -s

# Disabled parallel processing of target prerequisites to avoid that integration tests are racing each other (e.g. for ports) and may fail.
# Since we are not using this Makefile for compilation, limiting parallelism will not increase build time.
.NOTPARALLEL:

.PHONY: all build test clean unit_test unit_test_cover unit_test_race queryservice_test integration_test bson proto site_test site_integration_test docker_bootstrap docker_test docker_unit_test java_test php_test reshard_tests

all: build test

# Set a custom value for -p, the number of packages to be built/tested in parallel.
# This is currently only used by our Travis CI test configuration.
# (Also keep in mind that this value is independent of GOMAXPROCS.)
ifdef VT_GO_PARALLEL
VT_GO_PARALLEL := "-p" $(VT_GO_PARALLEL)
endif
# Link against the MySQL library in $VT_MYSQL_ROOT if it's specified.
ifdef VT_MYSQL_ROOT
# Clutter the env var only if it's a non-standard path.
  ifneq ($(VT_MYSQL_ROOT),/usr)
    CGO_LDFLAGS += -L$(VT_MYSQL_ROOT)/lib
  endif
endif

build:
	echo $$(date): Building source tree
	godep go install $(VT_GO_PARALLEL) -ldflags "$(tools/build_version_flags.sh)" ./go/...

# To pass extra flags, run test.go manually.
# For example: go run test.go -docker=false -- --extra-flag
# For more info see: go run test.go -help
test:
	go run test.go -docker=false

site_test: unit_test site_integration_test

clean:
	go clean -i ./go/...
	rm -rf third_party/acolyte

# This will remove object files for all Go projects in the same GOPATH.
# This is necessary, for example, to make sure dependencies are rebuilt
# when switching between different versions of Go.
clean_pkg:
	rm -rf ../../../../pkg Godeps/_workspace/pkg

unit_test:
	godep go test $(VT_GO_PARALLEL) ./go/...

# Run the code coverage tools, compute aggregate.
# If you want to improve in a directory, run:
#   go test -coverprofile=coverage.out && go tool cover -html=coverage.out
unit_test_cover:
	godep go test $(VT_GO_PARALLEL) -cover ./go/... | misc/parse_cover.py

unit_test_race:
	godep go test $(VT_GO_PARALLEL) -race ./go/...

# Run coverage and upload to coveralls.io.
# Requires the secret COVERALLS_TOKEN env variable to be set.
unit_test_goveralls:
	travis/goveralls.sh

queryservice_test:
	go run test.go -docker=false queryservice_vtocc queryservice_vttablet

.ONESHELL:
SHELL = /bin/bash

# Run the following tests after making worker changes.
worker_test:
	godep go test ./go/vt/worker/
	go run test.go -docker=false binlog resharding resharding_bytes vertical_split initial_sharding initial_sharding_bytes worker

site_integration_test:
	go run test.go -docker=false keyrange keyspace mysqlctl tabletmanager vtdb vtgatev2

java_test:
	godep go install ./go/cmd/vtgateclienttest
	mvn -f java/pom.xml clean verify

php_test:
	godep go install ./go/cmd/vtgateclienttest
	php $$PHP_FLAGS $$(which phpunit) php/tests

bson:
	go generate ./go/...

# This rule rebuilds all the go files from the proto definitions for gRPC.
# 1. list all proto files.
# 2. remove 'proto/' prefix and '.proto' suffix.
# 3. (go) run protoc for each proto and put in go/vt/proto/${proto_file_name}/
# 4. (python) run protoc for each proto and put in py/vtproto/
proto:
	find proto -name '*.proto' -print | sed 's/^proto\///' | sed 's/\.proto//' | xargs -I{} $$VTROOT/dist/protobuf/bin/protoc -Iproto proto/{}.proto --go_out=plugins=grpc:go/vt/proto/{}
	find go/vt/proto -name "*.pb.go" | xargs sed --in-place -r -e 's,import ([a-z0-9_]+) ".",import \1 "github.com/youtube/vitess/go/vt/proto/\1",g'
	find proto -name '*.proto' -print | sed 's/^proto\///' | sed 's/\.proto//' | xargs -I{} $$VTROOT/dist/protobuf/bin/protoc -Iproto proto/{}.proto --python_out=py/vtproto --grpc_out=py/vtproto --plugin=protoc-gen-grpc=$$VTROOT/dist/grpc/bin/grpc_python_plugin

# This rule builds the bootstrap images for all flavors.
docker_bootstrap:
	docker/bootstrap/build.sh common
	docker/bootstrap/build.sh mariadb
	docker/bootstrap/build.sh mysql56

docker_base:
	# Fix permissions before copying files, to avoid AUFS bug.
	chmod -R o=g *
	docker build -t vitess/base .

docker_lite:
	cd docker/lite && ./build.sh

docker_guestbook:
	cd examples/kubernetes/guestbook && ./build.sh

# This rule loads the working copy of the code into a bootstrap image,
# and then runs the tests inside Docker.
# Example: $ make docker_test flavor=mariadb
docker_test:
	go run test.go -flavor $(flavor)

docker_unit_test:
	go run test.go -flavor $(flavor) unit

# This can be used to rebalance the total average runtime of each group of
# tests in Travis. The results are saved in test/config.json, which you can
# then commit and push.
rebalance_tests:
	go run test.go -rebalance 5 -remote-stats http://enisoc.com:15123/travis/stats
