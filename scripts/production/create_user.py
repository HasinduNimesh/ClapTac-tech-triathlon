#!/usr/bin/env python3
"""Provision a ThunderID person and the matching Waypoint application profile.

Run as the VM administrator from the repository root. Never pass a password on
the command line: a random initial password is generated and written to a
private file on the VM.
"""

import argparse
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
import uuid

import yaml


ROOT = Path(__file__).resolve().parents[2]
PRIVATE_ROOT = Path.home() / ".config/waypoint/thunderid"
INVITATIONS = Path.home() / ".config/waypoint/new-user-credentials"
OU_ID = "01900000-0000-7000-8000-000000000001"
EMAIL_RE = re.compile(r"^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$")
PROFILE_TABLE = {
    "STORE_MANAGER": "shared.store_manager_profiles",
    "LOADER": "shared.loader_profiles",
    "DRIVER": "shared.driver_profiles",
}
PROFILE_COLUMN = {
    "STORE_MANAGER": "outlet_id",
    "LOADER": "depot",
    "DRIVER": "vehicle_id",
}


def run(command, *, input_text=None):
    return subprocess.run(command, input=input_text, text=True, check=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def psql(sql, **variables):
    command = [
        "sudo", "docker", "compose", "--env-file", ".env.production",
        "-f", "docker-compose.yml", "-f", "docker-compose.production.yml",
        "exec", "-T", "postgres", "psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1",
        "-U", "waypoint", "-d", "waypoint",
    ]
    for name, value in variables.items():
        command.extend(["-v", f"{name}={value}"])
    return run(command, input_text=sql).stdout.strip()


def existing_email(email):
    directory = PRIVATE_ROOT / "resources/users"
    paths = run(["sudo", "find", str(directory), "-maxdepth", "1", "-type", "f",
                 "-name", "*.yaml", "-print0"]).stdout.split("\0")
    for path in filter(None, paths):
        item = yaml.safe_load(run(["sudo", "cat", path]).stdout)
        attributes = item.get("attributes", {}) if isinstance(item, dict) else {}
        if str(attributes.get("email", "")).lower() == email:
            return True
    return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--email", required=True, help="Login username and email")
    parser.add_argument("--role", required=True,
                        choices=["DISPATCHER", "STORE_MANAGER", "LOADER", "DRIVER"])
    parser.add_argument("--name", required=True, help="Person's display name")
    parser.add_argument("--outlet-id", help="Required for STORE_MANAGER")
    parser.add_argument("--depot", help="Required for LOADER")
    parser.add_argument("--vehicle-id", help="Required for DRIVER")
    args = parser.parse_args()

    if os.geteuid() == 0:
        parser.error("Run as the VM administrator, without sudo; the script uses sudo where needed")
    email = args.email.strip().lower()
    name = args.name.strip()
    if not EMAIL_RE.fullmatch(email) or len(email) > 254:
        parser.error("Provide a valid email address")
    if not name or len(name) > 120:
        parser.error("Name must be 1 to 120 characters")
    detail = {"STORE_MANAGER": args.outlet_id, "LOADER": args.depot,
              "DRIVER": args.vehicle_id}.get(args.role)
    supplied = [v for v in (args.outlet_id, args.depot, args.vehicle_id) if v]
    if args.role == "DISPATCHER" and supplied:
        parser.error("DISPATCHER does not take an outlet, depot, or vehicle")
    if args.role != "DISPATCHER" and (not detail or len(supplied) != 1):
        parser.error("Provide exactly the matching --outlet-id, --depot, or --vehicle-id")
    if detail and (not detail.strip() or len(detail) > 100):
        parser.error("Profile detail must be 1 to 100 characters")
    if not (ROOT / ".env.production").is_file():
        parser.error("Run from the deployed repository with .env.production present")
    os.chdir(ROOT)

    if existing_email(email):
        parser.error("A ThunderID user with this email already exists")

    if args.role == "STORE_MANAGER":
        exists = psql("SELECT EXISTS (SELECT 1 FROM shared.outlets WHERE id = :'value');\n",
                      value=detail).splitlines()[-1].strip()
        if exists != "t":
            parser.error("Outlet ID does not exist in Waypoint")
    if args.role == "DRIVER":
        exists = psql("SELECT EXISTS (SELECT 1 FROM fleet.vehicles WHERE vehicle_id = :'value');\n",
                      value=detail).splitlines()[-1].strip()
        if exists != "t":
            parser.error("Vehicle ID does not exist in Waypoint")

    identity = uuid.uuid4().hex
    user_id = f"USR{identity[:12].upper()}"
    subject = f"waypoint-user-{identity}"
    password = secrets.token_hex(32)
    given_name, _, family_name = name.partition(" ")
    user = {
        "resource_type": "user", "id": f"waypoint-person-{identity}",
        "type": "Person", "ouId": OU_ID,
        "attributes": {
            "username": email, "email": email, "sub": subject,
            "name": name, "given_name": given_name, "family_name": family_name,
        },
        "credentials": {"password": password},
    }

    INVITATIONS.mkdir(mode=0o700, parents=True, exist_ok=True)
    INVITATIONS.chmod(0o700)
    credential_path = INVITATIONS / f"{user_id}.txt"
    credential_fd = os.open(credential_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(credential_fd, "w") as file:
        file.write(f"Username: {email}\nInitial password: {password}\n")

    staging = INVITATIONS / f"{user_id}.yaml"
    destination = PRIVATE_ROOT / "resources/users" / f"{user_id}.yaml"
    try:
        with staging.open("x") as file:
            yaml.safe_dump(user, file, sort_keys=False)
        staging.chmod(0o600)
        run(["sudo", "install", "-o", "10001", "-g", "10001", "-m", "600",
             str(staging), str(destination)])

        sql = "BEGIN;\n"
        sql += "INSERT INTO shared.users (id, identity_subject, role) VALUES (:'user_id', :'subject', :'role');\n"
        if args.role in PROFILE_TABLE:
            sql += (f"INSERT INTO {PROFILE_TABLE[args.role]} (user_id, {PROFILE_COLUMN[args.role]}) "
                    "VALUES (:'user_id', :'detail');\n")
        sql += "COMMIT;\n"
        try:
            psql(sql, user_id=user_id, subject=subject, role=args.role,
                 detail=detail or "")
        except subprocess.CalledProcessError:
            run(["sudo", "rm", "--", str(destination)])
            raise
    except Exception:
        credential_path.unlink(missing_ok=True)
        raise
    finally:
        staging.unlink(missing_ok=True)

    try:
        run(["sudo", "docker", "compose", "--env-file", ".env.production",
             "-f", "docker-compose.yml", "-f", "docker-compose.production.yml",
             "restart", "thunderid-prod"])
        for _ in range(20):
            response = subprocess.run(["curl", "-fsS", "--max-time", "2",
                                       "http://127.0.0.1:8091/health/readiness"],
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if response.returncode == 0:
                break
            import time
            time.sleep(2)
        else:
            raise RuntimeError("ThunderID did not become ready after restart")
    except Exception as error:
        print(f"User records created, but ThunderID needs attention: {error}", file=sys.stderr)
        print(f"Protected initial credential file: {credential_path}", file=sys.stderr)
        sys.exit(1)

    print(f"Created {email} as {args.role} ({user_id}).")
    print(f"Protected initial credential file: {credential_path}")
    print("Read it over SSH, deliver it privately, then remove the initial credential file.")


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print(f"Provisioning command failed: {error.cmd[0]} exited {error.returncode}",
              file=sys.stderr)
        sys.exit(1)
