# TODO

- Commit some volumes files containing configurations we want to persist
- Implement consistency checks (e.g. dnsmasq.conf vs docker-compose.yml addresses) (maybe a library might come out of it?)
- Do not crash when first running containers due to dead symlinks
- Clearly define the flow of things to set up before running docker compose up
- Streamline installation of nginx conf files along with apps
- Add health checks to services with `depends_on: { condition: service_healthy }` for proper startup order
- Add resource limits (CPU/memory) to containers to prevent resource exhaustion
- Set 600 permissions for sensitive files (like env files)
- Improve naming of docker resources and yaml fields and reduce namespace aliasing
- Check if it's possible to enforce volume existence when running docker compose
- Figure out latest best practice for docker vs host user ownership
- Evaluate whether to go back to unless-stopped restart policy now that we do not have `depends_on` in services
- Define hub.docker.com prefix for image urls to not assume docker registry as default
- Consider using ipvlan/macvlan/external networks instead of docker-managed ones to make hacks more explicit (or add an adr for why we stick to docker networks)
