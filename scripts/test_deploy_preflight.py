"""Guards the production deploy preflight in .github/workflows/ci-cd.yml.

The identity and credentials check is Python inside a single-quoted shell argument. One apostrophe in a
comment ends the quote early; the rest of the script is then passed as arguments and the audience check
compares against the wrong word, so every deploy fails. These tests pull the real script out of the workflow
and run it, so a change to it that breaks the quoting or the logic fails here first.
"""
import json
import pathlib
import subprocess
import sys
import textwrap
import unittest

WORKFLOW = pathlib.Path(__file__).resolve().parent.parent / ".github" / "workflows" / "ci-cd.yml"
OPEN = "dc config --format json | python3 -c '\n"
CLOSE = "' \"$audience\""
AUDIENCE = "https://waypoint.claptac.dev/api/v1"
ISSUER = "https://id.waypoint.claptac.dev"


def preflight_script() -> str:
    text = WORKFLOW.read_text()
    start = text.index(OPEN) + len(OPEN)
    end = text.index(CLOSE, start)
    return textwrap.dedent(text[start:end])


def services(**changes):
    shared_env = {
        "OIDC_ISSUER": ISSUER,
        "OIDC_AUDIENCE": AUDIENCE,
        "OIDC_JWKS_URL": ISSUER + "/oauth2/jwks",
        "OIDC_TOKEN_URL": ISSUER + "/oauth2/token",
        "DATABASE_URL": "postgres://waypoint_shared:s3cret@postgres:5432/waypoint?sslmode=disable",
    }
    shared_env.update(changes)
    return {"services": {
        "shared-service": {"environment": shared_env},
        "nginx": {"ports": [{"host_ip": "127.0.0.1", "published": "8080", "target": 80}]},
    }}


def run(config):
    return subprocess.run([sys.executable, "-c", preflight_script(), AUDIENCE], input=json.dumps(config),
                          capture_output=True, text=True)


class DeployPreflight(unittest.TestCase):
    def test_script_has_no_single_quote(self):
        # A single quote inside would end the shell quoting that wraps the script.
        self.assertNotIn("'", preflight_script())

    def test_accepts_a_production_configuration(self):
        result = run(services())
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_refuses_the_wrong_audience(self):
        result = run(services(OIDC_AUDIENCE="waypoint-api"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("API audience", result.stderr)

    def test_refuses_the_default_database_password(self):
        result = run(services(DATABASE_URL="postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("default database password", result.stderr)

    def test_refuses_no_tls_to_a_remote_database(self):
        result = run(services(DATABASE_URL="postgres://u:p@db.example.com:5432/w?sslmode=disable"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sslmode=disable", result.stderr)


if __name__ == "__main__":
    unittest.main()
