"""A fake of NordVPN's server list, with exp's fake VPN as its one server.

Serves the endpoint of Nord's API that Gluetun downloads its server list from.
"""

import json
import os
import socket
import ssl
from functools import partial
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlsplit


def main() -> None:
    body = json.dumps(get_servers_json(os.environ["VPN_ADDRESS"], os.environ["VPN_PUBLIC_KEY"]))

    http_server = HTTPServer(("", 443), partial(ServersHandler, body))
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain("fake-vpn-servers.crt", "fake-vpn-servers.key")
    http_server.socket = context.wrap_socket(http_server.socket, server_side=True)
    http_server.serve_forever()


# Gluetun reads each server's details through the ids of the groups, services,
# locations and technologies listed next to the servers
# Ref: https://github.com/qdm12/gluetun/blob/v3.41.0/internal/provider/nordvpn/updater/models.go
def get_servers_json(vpn_address: str, vpn_public_key: str) -> dict[str, object]:
    return {
        "servers": [
            {
                "name": "Italy #1",
                "station": vpn_address,
                "hostname": "it1.nordvpn.com",
                "status": "online",
                "location_ids": [1],
                "group_ids": [1],
                "service_ids": [1],
                "ips": [{"type": "entry", "ip": {"ip": vpn_address}}],
                "technologies": [
                    {
                        "id": 1,
                        "status": "online",
                        "metadata": [{"name": "public_key", "value": vpn_public_key}],
                    }
                ],
            }
        ],
        "groups": [{"id": 1, "title": "Europe", "type": {"identifier": "regions"}}],
        "services": [{"id": 1, "identifier": "vpn"}],
        "locations": [{"id": 1, "country": {"name": "Italy", "city": {"name": "Milan"}}}],
        "technologies": [{"id": 1, "identifier": "wireguard_udp"}],
    }


class ServersHandler(BaseHTTPRequestHandler):
    def __init__(
        self,
        body: str,
        request: socket.socket,
        client_address: tuple[str, int],
        server: HTTPServer,
    ) -> None:
        # The base constructor handles the request, so the field has to be set first
        self._body = body
        super().__init__(request, client_address, server)

    def do_GET(self) -> None:
        if urlsplit(self.path).path != "/v2/servers":
            self.send_error(404)
            return
        encoded_body = self._body.encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(encoded_body)))
        self.end_headers()
        self.wfile.write(encoded_body)


if __name__ == "__main__":
    main()
