#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Shared SSRF-guard utilities.

Uses only the standard library so it can be imported from both ``api/`` and
``common/`` without pulling in any heavyweight dependencies.
"""
import ipaddress
import logging
import socket
from urllib.parse import urlparse

logger = logging.getLogger(__name__)

_DEFAULT_ALLOWED_SCHEMES: frozenset[str] = frozenset({"http", "https"})


def _effective_ip(
    ip: ipaddress.IPv4Address | ipaddress.IPv6Address,
) -> ipaddress.IPv4Address | ipaddress.IPv6Address:
    """Return the IPv4 equivalent for IPv4-mapped IPv6 addresses, unchanged otherwise.

    Without this normalization ``::ffff:127.0.0.1`` would pass ``is_global``
    as an IPv6Address in some Python versions, bypassing the loopback check.
    """
    if isinstance(ip, ipaddress.IPv6Address):
        mapped = ip.ipv4_mapped
        if mapped is not None:
            return mapped
    return ip


def assert_url_is_safe(
    url: str,
    *,
    allowed_schemes: frozenset[str] = _DEFAULT_ALLOWED_SCHEMES,
) -> tuple[str, str]:
    """Raise ``ValueError`` if *url* is not safe to fetch (SSRF guard).

    Checks performed in order:

    1. Scheme is in *allowed_schemes*.
    2. Hostname is present.
    3. **Every** address returned by ``getaddrinfo`` is globally routable
       (``ip.is_global``).  This is an allowlist approach: it catches private,
       loopback, link-local, reserved, multicast, and all other
       special-purpose ranges rather than individual deny-list flags.
       IPv4-mapped IPv6 addresses (e.g. ``::ffff:127.0.0.1``) are normalised
       to their IPv4 form via :func:`_effective_ip` before the check.

    Returns ``(hostname, resolved_ip)`` — the first validated public IP string
    — so the caller can **pin** that address in its HTTP client and prevent
    DNS-rebinding attacks (the hostname is resolved exactly once).
    """
    parsed = urlparse(url)
    scheme = parsed.scheme
    if scheme not in allowed_schemes:
        logger.warning(
            "SSRF guard blocked URL with disallowed scheme: scheme=%r url=%r",
            scheme,
            url,
        )
        raise ValueError(
            f"Disallowed URL scheme: {scheme!r}. "
            f"Only {sorted(allowed_schemes)} are allowed."
        )

    hostname = parsed.hostname
    if not hostname:
        logger.warning("SSRF guard blocked URL with missing host: url=%r", url)
        raise ValueError("URL is missing a host.")

    try:
        addr_infos = socket.getaddrinfo(hostname, None)
    except socket.gaierror as exc:
        logger.warning(
            "SSRF guard could not resolve hostname=%r reason=%s", hostname, exc
        )
        raise ValueError(
            f"Could not resolve hostname {hostname!r}: {exc}"
        ) from exc

    resolved_ip: str | None = None
    for _family, _type, _proto, _canonname, sockaddr in addr_infos:
        raw_ip = ipaddress.ip_address(sockaddr[0])
        eff_ip = _effective_ip(raw_ip)
        if not eff_ip.is_global:
            logger.warning(
                "SSRF guard blocked URL: hostname=%r resolved to non-public address=%s",
                hostname,
                raw_ip,
            )
            raise ValueError(
                f"URL resolves to a non-public address ({raw_ip}), which is not allowed."
            )
        if resolved_ip is None:
            resolved_ip = str(raw_ip)

    if resolved_ip is None:
        logger.warning(
            "SSRF guard blocked URL: hostname=%r resolved to no addresses", hostname
        )
        raise ValueError(f"Hostname {hostname!r} resolved to no addresses.")

    return hostname, resolved_ip
