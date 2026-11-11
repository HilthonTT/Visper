import ipaddress

import httpx
import pytest

from visper_ai.core.utils import webhook
from visper_ai.core.utils.webhook import WebhookURLError, is_public_ip, post_webhook, validate_webhook_url


@pytest.mark.parametrize(
    ("ip", "public"),
    [
        ("8.8.8.8", True),
        ("2606:4700:4700::1111", True),
        ("127.0.0.1", False),
        ("10.1.2.3", False),
        ("172.16.0.1", False),
        ("192.168.1.1", False),
        ("169.254.169.254", False),  # cloud metadata
        ("100.64.0.1", False),  # carrier-grade NAT
        ("0.0.0.0", False),
        ("224.0.0.1", False),
        ("::1", False),
        ("fd00::1", False),
        ("::ffff:127.0.0.1", False),  # IPv4-mapped loopback
    ],
)
def test_is_public_ip(ip, public):
    assert is_public_ip(ipaddress.ip_address(ip)) is public


@pytest.mark.parametrize(
    "url",
    [
        "ftp://8.8.8.8/x",
        "http://user:pw@8.8.8.8/x",
        "http:///nohost",
        "http://127.0.0.1/x",
        "http://8.8.8.8:99999/x",
    ],
)
async def test_validate_webhook_url_rejects(url):
    with pytest.raises(WebhookURLError):
        await validate_webhook_url(url)


async def test_validate_webhook_url_accepts_public_ip_literal():
    await validate_webhook_url("https://8.8.8.8/hook")


async def test_private_networks_can_be_allowed_explicitly():
    await validate_webhook_url("http://127.0.0.1:9000/hook", allow_private=True)


async def test_post_webhook_connects_to_the_validated_ip(monkeypatch):
    """DNS rebinding: a second resolution answering 127.0.0.1 must never be used."""
    answers = iter([[ipaddress.ip_address("93.184.216.34")], [ipaddress.ip_address("127.0.0.1")]])

    async def fake_resolve(host, port):
        return next(answers)

    monkeypatch.setattr(webhook, "_resolve", fake_resolve)
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(204)

    await post_webhook(
        "https://hooks.example.com:8443/cb?x=1",
        {"status": "completed"},
        http_timeout=httpx.Timeout(1.0),
        transport=httpx.MockTransport(handler),
    )

    (request,) = seen
    assert str(request.url) == "https://93.184.216.34:8443/cb?x=1"
    assert request.headers["host"] == "hooks.example.com:8443"
    assert request.extensions["sni_hostname"] == "hooks.example.com"


async def test_post_webhook_refuses_private_resolution(monkeypatch):
    async def fake_resolve(host, port):
        return [ipaddress.ip_address("10.0.0.5")]

    monkeypatch.setattr(webhook, "_resolve", fake_resolve)
    transport = httpx.MockTransport(lambda request: pytest.fail("must not send"))
    with pytest.raises(WebhookURLError):
        await post_webhook("http://evil.example/cb", {}, http_timeout=httpx.Timeout(1.0), transport=transport)
