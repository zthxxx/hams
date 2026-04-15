# Dev Sandbox — Spec Delta

## MODIFIED Requirements

### Requirement: Sudo policy in dev sandbox

The sudoers policy baked into the template Dockerfile SHALL grant passwordless sudo to any resolvable user (`ALL ALL=(ALL) NOPASSWD: ALL`). This is scoped to dev-sandbox images only; it is not a general hams security posture.

#### Scenario: Hams apply succeeds end-to-end via sudo

- **WHEN** an example's hamsfile declares an `apt` package (e.g., `htop`)
- **AND** the developer runs `docker exec hams-<example> hams apply`
- **THEN** hams invokes `sudo apt-get install -y htop` inside the container
- **AND** the command succeeds
- **AND** resulting `.state/<machine_id>/apt.state.yaml` is written with state `ok`
- **AND** the state file on the host is owned by the host uid (not root, not uid 1000)
