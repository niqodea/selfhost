# TODO

- Commit some volumes files containing configurations we want to persist
- Implement consistency checks (e.g. dnsmasq.conf vs docker-compose.yml addresses) (maybe a library might come out of it?)
- Do not crash when first running containers due to dead symlinks
- Clearly define the flow of things to set up before running docker compose up
- Streamline installation of nginx conf files along with apps
- Add health checks to services with `depends_on: { condition: service_healthy }` for proper startup order
- Add resource limits (CPU/memory) to containers to prevent resource exhaustion
- Set 600 permissions for sensitive files (like env files)
- Check if it makes sense to use native images instead of those provided by linuxserver.io (what do they add?)
- Review usage of `HOST_NETWORK_SUBNET`
- Improve naming of docker resources and yaml fields and reduce namespace aliasing
- Check if it's possible to enforce volume existence when running docker compose
