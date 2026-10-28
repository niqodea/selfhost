# ADR: Gluetun - Single Centralized Gateway

**Decision:** Use one Gluetun instance as a shared VPN gateway via host iptables.

## Context

Previously ran separate Gluetun instances per service stack due to Docker's inability to designate a container as a network gateway.

## Solution

Centralize routing at the host level.
Create a dedicated Docker network (`gluetun-network`) and configure host iptables to route all traffic from its bridge interface (`br-gluetun`) through the Gluetun container.

Containers needing VPN:
1. Join `gluetun-network` with `gw_priority: 1` (makes it default route)
2. Join other networks for inbound connectivity (since Docker port forwarding breaks when routed through VPN)

Inbound port forwarding is handled manually via host iptables, targeting the container's non-VPN network IP.
This is critical for `wireguard-wan`, as Docker's `ports:` directive routes through the `gw_priority` network (`gluetun-network`), so responses would exit via VPN and never reach clients.

## Why This Works

Docker can't assign a container as a network's gateway ([moby/moby#48193](https://github.com/moby/moby/issues/48193), [moby/moby#20758](https://github.com/moby/moby/issues/20758)).
But host iptables can intercept traffic at the bridge interface level, bypassing Docker's routing entirely.
The complexity moves from multiple Gluetun instances to a single iptables configuration.

## Trade-offs

| Pros                 | Cons                                 |
|----------------------|--------------------------------------|
| Single VPN instance  | Requires host iptables configuration |
| No port collisions   | Port forwarding needs manual setup   |
| Lower resource usage |                                      |

## Conclusion

Host iptables centralization beats per-service Gluetun instances.
