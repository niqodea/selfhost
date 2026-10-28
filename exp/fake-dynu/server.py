"""A fake Dynu for one domain.

Serves the IP update and TXT record endpoints of Dynu's API, its nameserver,
and ipify's public IP.
"""

import json
import os
import selectors
import socket
import ssl
import struct
from dataclasses import dataclass
from enum import IntEnum
from functools import partial
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlsplit


def main() -> None:
    zone = Zone(domain=os.environ["DOMAIN"])

    http_server = HTTPServer(("", 443), partial(DynuHandler, zone, os.environ["PUBLIC_IP"]))
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain("fake-dynu.crt", "fake-dynu.key")
    http_server.socket = context.wrap_socket(http_server.socket, server_side=True)

    dns_socket = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    dns_socket.bind(("", 53))

    # Both servers run on this one thread, so the zone needs no lock
    with selectors.DefaultSelector() as selector:
        selector.register(http_server, selectors.EVENT_READ)
        selector.register(dns_socket, selectors.EVENT_READ)
        while True:
            for key, _ in selector.select():
                if key.fileobj is http_server:
                    http_server.handle_request()
                else:
                    query, address = dns_socket.recvfrom(512)
                    dns_socket.sendto(answer_dns_query(query, zone), address)


@dataclass(frozen=True)
class TxtRecord:
    node_name: str
    text: str


class Zone:
    def __init__(self, domain: str) -> None:
        self.domain = domain
        self._ipv4_address: str | None = None
        self._txt_records: dict[int, TxtRecord] = {}
        self._next_id = 1

    def get_ipv4_address(self) -> str | None:
        return self._ipv4_address

    def set_ipv4_address(self, ipv4_address: str) -> None:
        self._ipv4_address = ipv4_address

    def get_txt_records(self) -> dict[int, TxtRecord]:
        return dict(self._txt_records)

    def add_txt_record(self, record: TxtRecord) -> int:
        id_ = self._next_id
        self._next_id += 1
        self._txt_records[id_] = record
        return id_

    def remove_txt_record(self, id_: int) -> None:
        del self._txt_records[id_]

    def get_hostname(self, node_name: str) -> str:
        if node_name == "":
            return self.domain
        return f"{node_name}.{self.domain}"


class DynuHandler(BaseHTTPRequestHandler):
    def __init__(
        self,
        zone: Zone,
        public_ip: str,
        request: socket.socket,
        client_address: tuple[str, int],
        server: HTTPServer,
    ) -> None:
        # The base constructor handles the request, so the fields have to be set first
        self._zone = zone
        self._public_ip = public_ip
        super().__init__(request, client_address, server)

    def do_GET(self) -> None:
        url = urlsplit(self.path)
        match url.path.strip("/").split("/"):
            case [""]:
                self._respond(self._public_ip)
            case ["nic", "update"]:
                query = parse_qs(url.query)
                if query["hostname"] != [self._zone.domain]:
                    self.send_error(404)
                    return
                self._zone.set_ipv4_address(query["myip"][0])
                self._respond("good")
            case ["v2", "dns", "getroot", hostname] if hostname == self._zone.domain:
                # The zone is the only domain, so its id is always 1
                self._respond_json(
                    {"statusCode": 200, "id": 1, "domainName": hostname, "hostname": hostname, "node": ""}
                )
            case ["v2", "dns", "1", "record"]:
                records = [
                    self._get_record_json(id_, record)
                    for id_, record in self._zone.get_txt_records().items()
                ]
                self._respond_json({"statusCode": 200, "dnsRecords": records})
            case _:
                self.send_error(404)

    def do_POST(self) -> None:
        match urlsplit(self.path).path.strip("/").split("/"):
            case ["v2", "dns", "1", "record"]:
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                if body["recordType"] != "TXT":
                    self.send_error(400, "Only TXT records are supported")
                    return
                record = TxtRecord(node_name=body["nodeName"], text=body["textData"])
                id_ = self._zone.add_txt_record(record)
                self._respond_json({"statusCode": 200, **self._get_record_json(id_, record)})
            case _:
                self.send_error(404)

    def do_DELETE(self) -> None:
        match urlsplit(self.path).path.strip("/").split("/"):
            case ["v2", "dns", "1", "record", id_]:
                self._zone.remove_txt_record(int(id_))
                self._respond_json({"statusCode": 200})
            case _:
                self.send_error(404)

    def _get_record_json(self, id_: int, record: TxtRecord) -> dict[str, object]:
        return {
            "id": id_,
            "recordType": "TXT",
            "nodeName": record.node_name,
            "hostname": self._zone.get_hostname(record.node_name),
            "textData": record.text,
        }

    def _respond_json(self, body: dict[str, object]) -> None:
        self._respond(json.dumps(body))

    def _respond(self, body: str) -> None:
        encoded_body = body.encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(encoded_body)))
        self.end_headers()
        self.wfile.write(encoded_body)


class RecordType(IntEnum):
    A = 1
    SOA = 6
    TXT = 16


# Ref: https://www.rfc-editor.org/rfc/rfc1035#section-4.1
def answer_dns_query(query: bytes, zone: Zone) -> bytes:
    id_, flags = struct.unpack("!HH", query[:4])

    labels: list[str] = []
    offset = 12
    while query[offset] != 0:
        length = query[offset]
        labels.append(query[offset + 1 : offset + 1 + length].decode())
        offset += 1 + length
    name = ".".join(labels)
    (type_,) = struct.unpack("!H", query[offset + 1 : offset + 3])
    question = query[12 : offset + 5]

    match type_:
        case RecordType.A:
            ipv4_address = zone.get_ipv4_address()
            if name == zone.domain and ipv4_address is not None:
                datas = [socket.inet_aton(ipv4_address)]
            else:
                datas = []
        case RecordType.SOA:
            if name == zone.domain:
                # Callers only look for which name has an SOA, to find the
                # zone, so its servers, address and timers are left empty
                datas = [b"\x00" + b"\x00" + struct.pack("!5I", 0, 0, 0, 0, 0)]
            else:
                datas = []
        case RecordType.TXT:
            datas = [
                bytes([len(record.text)]) + record.text.encode()
                for record in zone.get_txt_records().values()
                if zone.get_hostname(record.node_name) == name
            ]
        case _:
            datas = []

    # An authoritative response, that keeps the query's recursion flag
    response_flags = 0x8400 | (flags & 0x0100)
    header = struct.pack("!6H", id_, response_flags, 1, len(datas), 0, 0)
    # Each answer names the question's name by pointing at it, 12 bytes in, and
    # has a zero TTL, so no resolver keeps a record the API has changed
    answers = b"".join(
        struct.pack("!HHHIH", 0xC000 | 12, type_, 1, 0, len(data)) + data for data in datas
    )
    return header + question + answers


if __name__ == "__main__":
    main()
