import asyncio
import time
import unittest

import jwt
from cryptography.hazmat.primitives.asymmetric import rsa

from assistants.auth import Authenticator, AuthError

KEY = rsa.generate_private_key(public_exponent=65537, key_size=2048)
OTHER = rsa.generate_private_key(public_exponent=65537, key_size=2048)


class StaticJWKS:
    class _Key:
        key = KEY.public_key()

    def get_signing_key_from_jwt(self, token):
        return self._Key()


def token(key=KEY, **claims):
    body = {"sub": "store", "iss": "http://issuer", "aud": "waypoint-api", "exp": int(time.time()) + 60, **claims}
    return jwt.encode(body, key, algorithm="RS256")


class AuthenticatorTest(unittest.TestCase):
    auth = Authenticator("", "http://issuer", "waypoint-api", jwks_client=StaticJWKS())

    def subject(self, raw):
        return asyncio.run(self.auth.subject(raw))

    def test_valid_token(self):
        self.assertEqual(self.subject(token()), "store")

    def test_rejects_wrong_signature_issuer_audience_and_expiry(self):
        for raw in (token(OTHER), token(iss="http://evil"), token(aud="other"), token(exp=int(time.time()) - 5), ""):
            with self.assertRaises(AuthError):
                self.subject(raw)

    def test_rejects_unsigned_token(self):
        unsigned = jwt.encode({"sub": "store", "exp": int(time.time()) + 60}, None, algorithm="none")
        with self.assertRaises(AuthError):
            self.subject(unsigned)


if __name__ == "__main__":
    unittest.main()
