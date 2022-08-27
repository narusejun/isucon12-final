HOSTNAME:=$(shell hostname)
BRANCH:=master

.PHONY: deploy
deploy: checkout start

.PHONY: checkout
checkout:
	git fetch && \
	git reset --hard origin/$(BRANCH)  && \
	git switch -C $(BRANCH) origin/$(BRANCH)

.PHONY: start
start:
	cd common && ./deploy.sh
	cd $(HOSTNAME) && ./deploy.sh
