"""Human identity, same contract as pkg/auth and the agent-orchestrator.

The bearer token is verified against the ThunderID JWKS (RS256, issuer,
audience, exp, sub). Roles and outlet scope then come from shared-service
/profiles/me, never from token claims.
"""

from dataclasses import dataclass, field

import jwt
from starlette.concurrency import run_in_threadpool

from .business import BusinessClient, BusinessError


class AuthError(Exception):
    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status


@dataclass(frozen=True)
class Profile:
    user_id: str
    subject: str
    roles: list[str] = field(default_factory=list)
    outlet_ids: list[str] = field(default_factory=list)

    def has_role(self, role: str) -> bool:
        return role in self.roles


def bearer_token(header: str | None) -> str:
    if not header or not header.startswith("Bearer "):
        return ""
    return header[len("Bearer "):].strip()


class Authenticator:
    def __init__(self, jwks_url: str, issuer: str, audience: str, jwks_client=None):
        self._issuer = issuer
        self._audience = audience
        self._jwks = jwks_client or (jwt.PyJWKClient(jwks_url, cache_keys=True, lifespan=300, timeout=5) if jwks_url else None)

    async def subject(self, token: str) -> str:
        if not token:
            raise AuthError(401, "missing bearer token")
        if self._jwks is None:
            raise AuthError(401, "JWKS is not configured")
        try:
            key = await run_in_threadpool(self._jwks.get_signing_key_from_jwt, token)
            claims = jwt.decode(
                token,
                key.key,
                algorithms=["RS256"],
                audience=self._audience or None,
                issuer=self._issuer or None,
                options={"require": ["exp", "sub"], "verify_aud": bool(self._audience)},
            )
        except jwt.PyJWTError as exc:
            raise AuthError(401, "invalid access token") from exc
        sub = claims.get("sub")
        if not isinstance(sub, str) or not sub:
            raise AuthError(401, "token missing sub")
        return sub


async def resolve_profile(client: BusinessClient, token: str, subject: str) -> Profile:
    try:
        body = await client.get_json(client.shared_url, "/api/v1/shared/profiles/me", token)
    except BusinessError as exc:
        raise AuthError(403, "application profile not found") from exc
    raw = body.get("profile") or {}
    if raw.get("subject") and raw.get("subject") != subject:
        raise AuthError(403, "profile subject mismatch")
    return Profile(
        user_id=str(raw.get("userId", "")),
        subject=subject,
        roles=[str(r) for r in raw.get("roles") or []],
        outlet_ids=[str(o) for o in raw.get("outletIds") or []],
    )
