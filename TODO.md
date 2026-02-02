# TODO

- Commit some volumes files containing configurations we want to persist
- Implement consistency checks (e.g. dnsmasq.conf vs docker-compose.yml addresses) (maybe a library might come out of it?)
- Do not crash when first running containers due to dead symlinks
- Clearly define the flow of things to set up before running docker compose up
- Add health checks to services with `depends_on: { condition: service_healthy }` for proper startup order
- Add resource limits (CPU/memory) to containers to prevent resource exhaustion
- Set 600 permissions for sensitive files (like env files) (maybe update plug util for that)
- Check if it's possible to enforce volume existence when running docker compose
- Figure out latest best practice for docker vs host user ownership
- Evaluate whether to go back to unless-stopped restart policy now that we do not have `depends_on` in services
- Consider using ipvlan/macvlan/external networks instead of docker-managed ones to make hacks more explicit (or add an adr for why we stick to docker networks)
- Consider usage of list-like include files to add/remove apps with simple sed commands (Caddyfile, root docker-compose.yaml)
