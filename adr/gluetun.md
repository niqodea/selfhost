# ADR: Gluetun - Multiple Instances vs Single Gateway

**Decision:** Keep separate Gluetun instances per service stack.

## Context

Tried to consolidate multiple Gluetun containers into one shared VPN gateway. The idea was to have other containers use Gluetun's IP as their default gateway.

## Why It Doesn't Work

**Docker doesn't support using a container as a gateway.** The `--gateway` flag assigns an IP to the bridge interface, not to a container. This is a fundamental Docker limitation, not a configuration issue.

Relevant issues (all unresolved):
- [moby/moby#48193](https://github.com/moby/moby/issues/48193) — "Address already in use" when trying to assign gateway IP to container
- [moby/moby#20758](https://github.com/moby/moby/issues/20758) — Feature request from 2016, still open

## Alternatives Considered

| Approach                        | Problem                                                                             |
|---------------------------------|-------------------------------------------------------------------------------------|
| `network_mode: service:gluetun` | Port collisions — all services share one network namespace, so ports must be unique |
| SOCKS5/HTTP proxy               | Not all apps support it; doesn't capture all traffic; DNS leaks                     |
| Host iptables hacks             | Fragile, doesn't survive restarts                                                   |

The Gluetun maintainer [has confirmed](https://github.com/qdm12/gluetun/discussions/1084) port collision is unavoidable with shared network mode.

## Conclusion

Multiple Gluetun instances is the intended pattern. The memory cost (~55MB each with tuning) is acceptable. Port-number-as-service-identifier is an anti-pattern worth avoiding.
