SCRIPTS_DIR := ./scripts

.PHONY: default install dev build build-docker start test version release

default: install dev

install:
	@$(SCRIPTS_DIR)/install.sh

dev:
	@$(SCRIPTS_DIR)/dev.sh

build:
	@$(SCRIPTS_DIR)/build.sh

start:
	@$(SCRIPTS_DIR)/start.sh

test:
	@$(SCRIPTS_DIR)/test.sh

version:
	@$(SCRIPTS_DIR)/version.sh

build-docker:
	@$(SCRIPTS_DIR)/build-docker.sh

release:
	@$(SCRIPTS_DIR)/release.sh
