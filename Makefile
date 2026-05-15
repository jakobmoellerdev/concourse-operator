# Compatibility shim — delegates to Taskfile.
# Install go-task: https://taskfile.dev/installation/

TASK := $(shell command -v task 2>/dev/null)

ifndef TASK
  $(error "task (go-task) is not installed. See https://taskfile.dev/installation/")
endif

%:
	@task $@

.DEFAULT_GOAL := default
default:
	@task
