# ADR: Systemd-Managed Startup with Pre-Docker Failsafe Routing

**Decision:** Use systemd to configure a failsafe blackhole route before Docker starts, then run a startup script that handles app-specific routes and containers.

## Context

Containers routing through Gluetun require host-level ip rules and iptables configuration.
These aren't persistent across reboots and must be in place before containers start, or traffic leaks.

We use the `on-failure` restart policy for runtime crash recovery since it's useful and hard to reimplement.
However, Docker's `on-failure` policy [restarts containers on daemon/system boot](https://github.com/moby/moby/issues/47846) if they exited with a non-zero code, which can happen during shutdown if a container doesn't handle SIGTERM gracefully or coincidentally errors as the system goes down.
This means Docker may auto-restart containers on boot before routes are ready.
Ideally we'd use a policy that only restarts on actual container failure (not on boot), but Docker doesn't support this behavior.

Docker restarts these containers one by one, without `depends_on`, and `docker compose up` later leaves a running container alone, so a container can also start before what it depends on.
Each container therefore copes with its dependencies starting late, by retrying, or by exiting so that `on-failure` restarts it.

Docker has no pre-start hooks. Without them, `unless-stopped`/`always` policies offer no advantage over managing startup ourselves via systemd.

## Solution

### selfh-route-failsafe (Before Docker)

Configures a blackhole on the Gluetun network so any prematurely-started container drops traffic rather than leaking it.

A systemd drop-in makes `docker.service` depend on this unit (`Requires=` + `After=`), ensuring the failsafe is always active before Docker can start containers.

### selfh-startup (After Docker)

For each app: runs `./route` if present, then `docker compose up --detach`.
App-specific routes are configured immediately before their containers start.

Technically, since this runs after `docker.service`, there's a race condition where rogue containers might start before app-specific routes are configured.
However, the upstream blackhole failsafe ensures traffic is safely dropped rather than leaked during this window.

## Boot Sequence

1. `selfh-route-failsafe` -> blackhole active
2. `docker.service` starts -> rogue containers hit blackhole
3. `selfh-startup` -> configures routes, starts containers properly

## Trade-offs

| Pros                             | Cons                                  |
|----------------------------------|---------------------------------------|
| Traffic safety via blackhole     | Two systemd services                  |
| Runtime crash recovery preserved | Failsafe must complete before Docker  |
|                                  | Works around Docker policy limitation |
|                                  | Boot restarts skip `depends_on`       |
