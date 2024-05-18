# Disable all the default make stuff
MAKEFLAGS += --no-builtin-rules
.SUFFIXES:

## Display a list of the documented make targets
.PHONY: help
help:
	@echo Documented Make targets:
	@perl -e 'undef $$/; while (<>) { while ($$_ =~ /## (.*?)(?:\n# .*)*\n.PHONY:\s+(\S+).*/mg) { printf "\033[36m%-30s\033[0m %s\n", $$2, $$1 } }' $(MAKEFILE_LIST) | sort

# ------------------------------------------------------------------------------
# NON-PHONY TARGETS
# ------------------------------------------------------------------------------

client/swagger.json:
	curl https://app-api.beta.surehub.io/swagger/v1/swagger.json > $@
	jq '.paths."/api/auth/login".post.responses."200" |= {"content":{"application/json":{"schema":{"$$ref":"#/components/schemas/AuthLoginResponse"}}}} | .components.schemas.AuthLoginResponse |= {"type":"object","required":["data"],"properties":{"data":{"required":["user","token"],"properties":{"user":{"$$ref":"#/components/schemas/UserResource"},"token":{"type":"string"}}}}}' client/swagger.json > client/swagger.json.tmp
	jq 'walk( if type=="object" and .format and .format == "int32" then del(.format) else . end )' client/swagger.json.tmp > client/swagger.json

# ------------------------------------------------------------------------------
# PHONY TARGETS
# ------------------------------------------------------------------------------

.PHONY: .FORCE
.FORCE:

## Update the swagger.json spec
.PHONY: update-spec
update-spec:
	rm -fv client/swagger.json
	$(MAKE) client/swagger.json

.PHONY: generate
generate:
	go generate ./...
