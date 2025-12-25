# TODO

- Commit some volumes files containing configurations we want to persist
- Use a gateway container to easily encrypt traffic from multiple containers with a single gluetun container (this might be useful: https://stackoverflow.com/a/69055795)
- Implement consistency checks (e.g. dnsmasq.conf vs docker-compose.yml addresses) (maybe a library might come out of it?)
- Do not crash when first running containers due to dead symlinks
- Clearly define the flow of things to set up before running docker compose up
- Streamline installation of nginx conf files along with apps
