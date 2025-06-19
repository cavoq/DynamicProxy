import os
from proxy.http.parser import HttpParser
from proxy.http.proxy import HttpProxyBasePlugin
from proxy.http.url import Url
from typing import Optional

UPSTREAM_PROXY = os.getenv("UPSTREAM_PROXY")
EXCEPTIONS = os.getenv("PROXY_EXCEPTIONS", "")
EXCEPTIONS_SET = set(e.strip().lower()
                     for e in EXCEPTIONS.split(",") if e.strip())


class DynamicUpstreamProxyPlugin(HttpProxyBasePlugin):
    """
    Dynamic upstream proxy plugin for Proxy.py.
    """

    def before_upstream_connection(
        self, request: HttpParser
    ) -> Optional[tuple]:

        hostname = request.host.lower() if request.host else ""
        use_direct = any(hostname.endswith(exc) for exc in EXCEPTIONS_SET)

        if use_direct:
            return None

        if UPSTREAM_PROXY:
            url = Url.from_bytes(UPSTREAM_PROXY.encode('utf-8'))
            if url.scheme and url.hostname:
                return (url.hostname, url.port or 80)
            else:
                raise ValueError("Invalid UPSTREAM_PROXY format")

        return None
