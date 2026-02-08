# ADR: VPN Routing - Explicit Opt-In for Sensitive Traffic

**Decision:** Route only P2P and high-risk services through VPN; use direct internet connection for everything else.

## Context

Need to decide default routing policy with centralized Gluetun gateway: VPN-by-default vs. direct-by-default.

Service categories:
- **High-risk**: P2P applications (ISP monitoring, legal notices)
- **Low-risk**: Metadata services, update checks, API calls
- **Must-be-direct**: DDNS updater, VPN server endpoints, local discovery

## Solution

Direct connection by default. Explicitly route sensitive services to `gluetun-network`.

**VPN-routed:** P2P applications, anything that could trigger ISP throttling/notices

**Direct-routed:** Media metadata services, monitoring, DDNS, inbound VPN servers

## Why This Works

Commercial VPN shifts trust from ISP to VPN provider, but neither guarantees true privacy.

HTTPS already encrypts content; ISP sees only destination domains (not sensitive for legitimate API services).

VPN adds meaningful protection only for traffic patterns that trigger ISP action (P2P) or legal risk.

## Trade-offs

| Pros                                    | Cons                                            |
|-----------------------------------------|-------------------------------------------------|
| Lower latency, simpler debugging        | Requires conscious routing decision per service |
| VPN failure doesn't break core services | ISP visibility into connection patterns         |
| No DDNS/VPN conflicts                   | Must remember to route new P2P services         |

## Conclusion

Route through VPN only when there's concrete risk.
Reserve VPN for services where ISP visibility creates legal or throttling concerns.
