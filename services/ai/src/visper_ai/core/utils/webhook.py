"""Validation and delivery of user-supplied webhook URLs (SSRF protection).

A webhook makes the server send a request to an address the caller chose. Without
checks, a caller could aim it at loopback, the private network or a cloud metadata
endpoint (169.254.169.254) and reach services that are not meant to be public.
"""

import asyncio
import ipaddress
import socket
from typing import Any
from urllib.parse import urlsplit, urlunsplit

import httpx

IPAddress = ipaddress.IPv4Address | ipaddress.IPv6Address


class WebhookURLError(ValueError):
    """The webhook URL is not allowed."""


def is_public_ip(ip: IPAddress) -> bool:
    """True only for globally routable unicast addresses."""
    if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped is not None:
        ip = ip.ipv4_mapped  # ::ffff:127.0.0.1 is loopback, not public
    return ip.is_global and not ip.is_multicast


async def _resolve(host: str, port: int) -> list[IPAddress]:
    """Resolve ``host`` to its addresses, in resolver preference order, without duplicates."""
    infos = await asyncio.get_running_loop().getaddrinfo(host, port, type=socket.SOCK_STREAM)
    addresses = (ipaddress.ip_address(info[4][0].split("%", 1)[0]) for info in infos)
    return list(dict.fromkeys(addresses))


async def validate_webhook_url(url: str, *, allow_private: bool = False) -> list[IPAddress]:
    """Raise WebhookURLError unless every address the URL's host resolves to is public.

    Returns the validated addresses (empty when ``allow_private`` skips resolution).
    """
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
        return []

    try:
        addresses = await _resolve(host, port)
    except OSError as e:
        raise WebhookURLError("callback_url host could not be resolved") from e

    if not addresses or not all(is_public_ip(ip) for ip in addresses):
        raise WebhookURLError("callback_url must point to a public internet address")
    return addresses


async def post_webhook(
    url: str,
    payload: dict[str, Any],
    *,
    allow_private: bool = False,
    http_timeout: httpx.Timeout,
    transport: httpx.AsyncBaseTransport | None = None,
) -> None:
    """Validate ``url`` and POST ``payload`` to the address that was validated.

    The connection is pinned to the IP checked by ``validate_webhook_url``: letting
    httpx resolve the host again allowed DNS rebinding (a TTL-0 record answering a
    public IP to the check and 127.0.0.1 to the connect). The original host is kept
    in the Host header and as the TLS SNI / certificate hostname. Redirects stay
    disabled so a public URL cannot bounce the request to an internal one.

    Raises WebhookURLError or httpx.HTTPError.
    """
    addresses = await validate_webhook_url(url, allow_private=allow_private)

    request_url = url
    headers: dict[str, str] = {}
    extensions: dict[str, Any] = {}
    parts = urlsplit(url)
    host = parts.hostname or ""
    try:
        ipaddress.ip_address(host)
        is_ip_literal = True
    except ValueError:
        is_ip_literal = False

    if addresses and not is_ip_literal:
        ip = addresses[0]
        ip_host = f"[{ip}]" if ip.version == 6 else str(ip)
        netloc = ip_host if parts.port is None else f"{ip_host}:{parts.port}"
        request_url = urlunsplit(parts._replace(netloc=netloc))
        headers["Host"] = parts.netloc
        if parts.scheme == "https":
            extensions["sni_hostname"] = host

    async with httpx.AsyncClient(timeout=http_timeout, follow_redirects=False, transport=transport) as client:
        await client.post(request_url, json=payload, headers=headers, extensions=extensions)
