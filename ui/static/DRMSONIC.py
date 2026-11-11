import socket
import ssl

import requests
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.poolmanager import PoolManager


class TLS1_2_Adapter(HTTPAdapter):
    def init_poolmanager(self, connections, maxsize, block=False):
        self.poolmanager = PoolManager(
            num_pools=connections,
            maxsize=maxsize,
            block=block,
            ssl_version=ssl.PROTOCOL_TLSv1_2,
        )


class session:
    def __init__(self, bind="", proxy="", force_tls1_2=False):
        self.session = requests.Session()
        if bind != "":
            if "." not in bind:
                options = [(socket.SOL_SOCKET, socket.SO_BINDTODEVICE, bind.encode())]
                bind = ""
            else:
                options = []

            self.session.get_adapter("http://").init_poolmanager(
                connections=requests.adapters.DEFAULT_POOLSIZE,
                maxsize=requests.adapters.DEFAULT_POOLSIZE,
                source_address=(bind, 0),
                socket_options=options,
            )
            kwargs = {
                "connections": requests.adapters.DEFAULT_POOLSIZE,
                "maxsize": requests.adapters.DEFAULT_POOLSIZE,
                "source_address": (bind, 0),
                "socket_options": options,
            }
            if force_tls1_2:
                kwargs["ssl_version"] = ssl.PROTOCOL_TLSv1_2
            self.session.get_adapter("https://").init_poolmanager(**kwargs)
        elif force_tls1_2:
            self.session.mount("https://", TLS1_2_Adapter())

        if proxy != "":
            self.session.proxies = {"http": proxy, "https": proxy}

    def get_session(self):
        return self.session


def parse_params(params, name, default=""):
    for p in params:
        if p.split("=")[0] == name and len(p.split("=")) >= 2:
            return p[len(p.split("=")[0]) + 1 :]
    return default


def dns(url):
    return None
