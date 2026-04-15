# Dev Sandbox Template

This directory is the baseline fixture copied by `task dev EXAMPLE=<name>`
when `examples/<name>/` does not yet exist.

## Layout

```text
.template/
  Dockerfile                   # debian-slim + dev user with passwordless sudo
  config/
    hams.config.yaml           # global config (store_path=/workspace/store)
  store/
    hams.config.yaml           # store config (profile_tag=dev, machine_id=sandbox)
    dev/                       # profile directory; put hamsfiles here
      .gitkeep
  state/                       # hams writes .state artifacts here (git-tracked)
    .gitkeep
```

## Not Gitignored

Everything under `examples/<name>/` — including `state/` — is git-tracked so
an example captures the complete end-to-end story of a hams run. Only future
build/tool caches (e.g., `examples/*/.cache/`) would be excluded, and none
exist today.

## Overriding the Image

An example may ship its own `Dockerfile` to extend the baseline. The image
tag is always `hams-dev-<name>`. There is no arch branching inside the image
— `start-container.sh` creates `/usr/local/bin/hams` as a symlink to the
correct `hams-linux-<arch>` after `docker run`.
