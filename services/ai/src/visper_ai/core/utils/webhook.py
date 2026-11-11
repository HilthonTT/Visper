"""Validation of user-supplied webhook URLs (SSRF protection).

A webhook makes the server send a request to an address the caller chose. Without
checks, a caller could aim it at loopback, the private network or a cloud metadata
endpoint (169.254.169.254) and reach services that are not meant to be public.
"""

import asyncio
import ipaddress
import socket
from urllib.parse import urlsplit

IPAddress = ipaddress.IPv4Address | ipaddress.IPv6Address


class WebhookURLError(ValueError):
    """The webhook URL is not allowed."""


def is_public_ip(ip: IPAddress) -> bool:
    """True only for globally routable unicast addresses."""
    if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped is not None:
        ip = ip.ipv4_mapped  # ::ffff:127.0.0.1 is loopback, not public
    return ip.is_global and not ip.is_multicast


async def validate_webhook_url(url: str, *, allow_private: bool = False) -> None:
    """Raise WebhookURLError unless every address the URL's host resolves to is public."""
    parts = urlsplit(url)
    if parts.scheme not in ("http", "https"):
        raise WebhookURLError("callback_url must use http or https")
    if parts.username or parts.password:
        raise WebhookURLError("callback_url must not contain credentials")
    host = parts.hostname
    if not host:
        raise WebhookURLError("callback_url has no host")

    try:
        port = parts.port or (443 if parts.scheme == "https" else 80)
    except ValueError as e:
        raise WebhookURLError("callback_url has an invalid port") from e

    if allow_private:
        return

    try:
        infos = await asyncio.get_running_loop().getaddrinfo(host, port, type=socket.SOCK_STREAM)
    except OSError as e:
        raise WebhookURLError("callback_url host could not be resolved") from e

    addresses = {ipaddress.ip_address(info[4][0].split("%", 1)[0]) for info in infos}
    if not addresses or not all(is_public_ip(ip) for ip in addresses):
        raise WebhookURLError("callback_url must point to a public internet address")
