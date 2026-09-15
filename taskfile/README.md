This directory contains additional scripts for Taskfile.yml, which is the main task runner configuration file for this project.

`consumer.ps1` copies the public runnable examples into `.tmp/consumer`, a separate module that requires this repository through a local replace directive, and compiles them there. It fails when an example imports an internal package. `task all` runs it.
