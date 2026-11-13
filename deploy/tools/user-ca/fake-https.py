"""fake-https.py <cert.pem> <key.pem> <port>: a fake Jellyfin-like server over HTTPS on the loopback,
for measure-user-ca.sh. Invented values only; every request is logged to stderr as "METHOD path"."""
import http.server
import json
import ssl
import sys

FILM = {"Id": "f1", "Name": "Film Behind TLS", "Type": "Movie", "ProductionYear": 2001, "UserData": {"Played": False}}


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        sys.stderr.write(f"{self.command} {self.path.split('?')[0]}\n")
        sys.stderr.flush()

    def send(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def route(self):
        path = self.path.split("?")[0]
        if path == "/movies/System/Info/Public":
            return self.send(200, {"ProductName": "Fake Movies", "ServerName": "tls", "Version": "10.11.0"})
        if path == "/movies/Users/AuthenticateByName":
            return self.send(200, {"AccessToken": "invented-token", "User": {"Id": "u1", "Name": "demo"}})
        if path == "/movies/Users/u1/Items/Resume":
            return self.send(200, {"Items": [], "TotalRecordCount": 0})
        if path == "/movies/Users/u1/Items":
            return self.send(200, {"Items": [FILM], "TotalRecordCount": 1})
        if path == "/movies/Users/u1/Items/f1":
            return self.send(200, FILM)
        return self.send(404, {})

    def do_GET(self):
        self.route()

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length") or 0))
        self.route()


server = http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[3])), Handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(sys.argv[1], sys.argv[2])
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
