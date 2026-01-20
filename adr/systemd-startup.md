# ADR: Systemd-Managed Startup with On-Failure Restart Policy

**Decision:** Use systemd to orchestrate container startup order, with `on-failure` restart policy instead of `unless-stopped`.

## Context

Docker Compose's `depends_on` only controls initial startup order and doesn't affect restarts.
When using `unless-stopped` or `always` restart policies, Docker's restart mechanism bypasses `depends_on` entirely.

This creates race conditions: if the host reboots, containers restart in arbitrary order.
A service might start before its dependency (e.g., a database client before the database), causing failures or undefined behavior.

## Solution

Delegate startup orchestration to systemd:

1. Set all containers to `restart: on-failure` (not `unless-stopped`)
2. Create a systemd unit that runs after `docker.service`
3. The unit executes a startup script that iterates through apps and runs `docker compose up --detach`

On reboot:
- Docker starts but doesn't auto-restart containers (they exited cleanly, not from failure)
- Systemd triggers the startup script after Docker is ready
- Script starts stacks sequentially, honoring implicit dependency order

On crash:
- `on-failure` lets Docker handle immediate recovery
- No systemd intervention needed for transient failures

## Why This Works

The key insight: clean shutdown is not a failure.
Containers stopped during `systemctl stop` or reboot exit with code 0.
`on-failure` only triggers on non-zero exits, so Docker won't auto-restart them.

This gives us the best of both worlds:
- Systemd handles boot-time orchestration
- Docker handles runtime crash recovery

## Trade-offs

| Pros                               | Cons                                              |
|------------------------------------|---------------------------------------------------|
| Honors dependency order on boot    | Extra systemd configuration                       |
| Single source of truth for startup | Must remember `on-failure` vs `unless-stopped`    |
| Works with existing `depends_on`   | Script-based ordering (implicit, not declarative) |
| Crash recovery still automatic     |                                                   |

## Conclusion

Systemd as the boot-time orchestrator, Docker as the runtime supervisor.
Each tool handles what it's designed for.
