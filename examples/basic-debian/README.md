# basic-debian — Dev Sandbox Scenario

A minimal end-to-end scenario demonstrating the dev sandbox against a
Debian bookworm container. Covers two builtin providers:

- `apt`  — installs `htop` (an interactive process viewer).
- `bash` — sets a safe global git option.

## Run

```sh
task dev EXAMPLE=basic-debian
```

In another terminal:

```sh
task dev:shell EXAMPLE=basic-debian
# inside the container:
hams --version
hams apply
hams apt list
```

## Layout

Inherits the baseline `examples/.template/` structure. Only the contents
of `store/dev/*.hams.yaml` differentiate this scenario from the template.
