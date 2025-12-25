# TODO

- Commit some volumes files containing configurations we want to persist
- Use a gateway container to easily encrypt traffic from multiple containers with a single gluetun container (this might be useful: https://stackoverflow.com/a/69055795)
- Implement consistency checks (e.g. dnsmasq.conf vs docker-compose.yml addresses) (maybe a library might come out of it?)
- Do not crash when first running containers due to dead symlinks
- Clearly define the flow of things to set up before running docker compose up
- Streamline installation of nginx conf files along with apps
- Add health checks to services with `depends_on: { condition: service_healthy }` for proper startup order
- Add resource limits (CPU/memory) to containers to prevent resource exhaustion
- Set 600 permissions for sensitive files (like env files)

## Modernization: Replace SWAG with Caddy

**Goal:** Replace SWAG with Caddy for simpler reverse proxy + automatic SSL

**Why Caddy:**
- Automatic HTTPS built-in (zero SSL configuration needed)
- Single binary, single process (vs SWAG's full container)
- Config is ~75% smaller than equivalent nginx
- HTTP/3/QUIC support out of the box
- Written in Go (static binary, minimal dependencies)
- Auto cert issuance and renewal with no cron jobs

**Migration Plan:**

1. **Preparation**
   - Audit existing `.subdomain.conf` files (qbittorrent, radicale, git-http-backend)
   - Document upstream container names and ports from existing configs
   - Understand DuckDNS wildcard cert requirements (*.${DUCKDNS_SUBDOMAIN}.duckdns.org)

2. **Create Caddy App**
   - Create `apps/caddy/docker-compose.yaml` with official Caddy image
   - Ports: 80, 443 (and 443/udp for HTTP/3)
   - Networks: MAIN_NETWORK (static IP like SWAG) and SWAG_NETWORK (keep name for compatibility)
   - Volumes: Caddyfile, caddy_data (for certs), caddy_config

3. **Write Caddyfile**
   - Configure DuckDNS DNS challenge for wildcard cert
   - Convert each .subdomain.conf to Caddy reverse_proxy blocks
   - Example: `radicale.${DUCKDNS_SUBDOMAIN}.duckdns.org { reverse_proxy radicale:5232 }`
   - Set global options (TLS settings, log levels)

4. **Test with One Service**
   - Start with simplest service (radicale or vaultwarden)
   - Verify SSL cert issuance from Let's Encrypt
   - Test service accessibility and functionality
   - Monitor Caddy logs for issues

5. **Incremental Migration**
   - Migrate one service at a time
   - Keep SWAG running for unmigrated services
   - Update DNS or internal routing per service
   - Validate each before moving to next

6. **Cutover & Cleanup**
   - Once all services migrated, stop SWAG
   - Remove `apps/swag/` (archive first)
   - Update install scripts if needed
   - Document new Caddy setup in CLAUDE.md or README

**Resources:**
- Caddy official site: https://caddyserver.com/
- Caddy Docker image: https://hub.docker.com/_/caddy
- Caddy docs for reverse proxy: https://caddyserver.com/docs/caddyfile/directives/reverse_proxy
- DuckDNS module: https://github.com/caddy-dns/duckdns
