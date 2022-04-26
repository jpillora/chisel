#!/bin/bash
# TODO
gocompat compare \
	--go1compat \
	--log-level=debug \
	--git-refs=origin/master..$(git rev-parse --abbrev-ref HEAD) \
	./...
