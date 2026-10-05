#!/usr/bin/env python3
"""Create a full set of Waypoint accounts, one per role and scope, on the VM.

Run from the repository root on the VM as the administrator, without sudo:

    python3 scripts/production/seed_accounts.py            # dry run: prints the plan, changes nothing
    python3 scripts/production/seed_accounts.py --apply    # creates the accounts

Each account gets a random password. The list is printed once and saved to a private file
(mode 600) under ~/.config/waypoint/new-user-credentials/. Passwords are never passed on a
command line, written to git, or sent anywhere. Accounts whose email already exists are skipped,
so a second run is safe. All database rows go in one transaction.
"""

import argparse
import datetime
import os
import re
import secrets
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path

import yaml

PRIVATE_ROOT = Path.home() / ".config/waypoint/thunderid"
USERS_DIR = PRIVATE_ROOT / "resources/users"
CREDENTIALS_DIR = Path.home() / ".config/waypoint/new-user-credentials"
OU_ID = "01900000-0000-7000-8000-000000000001"
NORTH, SOUTH = "DEPOT_NORTH", "DEPOT_SOUTH"  # Peliyagoda, Kandy

# (name, role, scope) where scope is None, a depot code, an outlet id or a vehicle id.
ROSTER = [
    # Dispatchers: head office sees every depot, the other two start in one depot.
    ("Kasun Wijesinghe", "DISPATCHER", None),
    ("Dilani Perera", "DISPATCHER", NORTH),
    ("Ruwan Jayasuriya", "DISPATCHER", SOUTH),
    # Store managers across both depots and every kind of outlet access.
    ("Nimali Fernando", "STORE_MANAGER", "OUT003"),      # Peliyagoda, street, van only
    ("Chamara Silva", "STORE_MANAGER", "OUT015"),        # Peliyagoda, mall bay
    ("Ishara Gunawardena", "STORE_MANAGER", "OUT005"),   # Peliyagoda, rear dock
    ("Thilini Rajapaksa", "STORE_MANAGER", "OUT004"),    # Peliyagoda, street
    ("Pradeep Bandara", "STORE_MANAGER", "OUT076"),      # Kandy, street, van only
    ("Sanduni Herath", "STORE_MANAGER", "OUT089"),       # Kandy, mall bay
    ("Lakmal Dissanayake", "STORE_MANAGER", "OUT084"),   # Kandy, rear dock
    ("Menaka Senanayake", "STORE_MANAGER", "OUT091"),    # Kandy, street
    # Loaders, one per depot.
    ("Tharindu Madushanka", "LOADER", NORTH),
    ("Hasini Abeywickrama", "LOADER", SOUTH),
    # Drivers: a van and a lorry, each with and without a cooler, in both depots.
    # (damindu@claptac.dev already drives VEH037, the Peliyagoda ambient van.)
    ("Nuwan Kumara", "DRIVER", "VEH035"),        # Peliyagoda, van, cooler
    ("Sampath Weerasinghe", "DRIVER", "VEH007"),  # Peliyagoda, lorry, cooler
    ("Janaka Ratnayake", "DRIVER", "VEH008"),     # Peliyagoda, lorry, no cooler
    ("Asanka Rathnayake", "DRIVER", "VEH057"),    # Kandy, van, cooler
    ("Dinesh Wickramasinghe", "DRIVER", "VEH059"),  # Kandy, van, no cooler
    ("Roshan Gamage", "DRIVER", "VEH041"),        # Kandy, lorry, cooler
    ("Chaminda Liyanage", "DRIVER", "VEH046"),    # Kandy, lorry, no cooler
]

PROFILE = {
    "DISPATCHER": ("shared.dispatcher_profiles", "depot"),
    "STORE_MANAGER": ("shared.store_manager_profiles", "outlet_id"),
    "LOADER": ("shared.loader_profiles", "depot"),
    "DRIVER": ("shared.driver_profiles", "vehicle_id"),
}
SAFE = re.compile(r"^[A-Za-z0-9_-]+$")


def run(cmd, input_text=None):
    return subprocess.run(cmd, input=input_text, text=True, check=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout


def psql(sql):
    return run(["sudo", "docker", "compose", "exec", "-T", "postgres", "psql", "-X", "-A", "-t",
                "-v", "ON_ERROR_STOP=1", "-U", "waypoint", "-d", "waypoint"], sql).strip()


def email_for(name):
    first, *rest = name.lower().split()
    return f"{first}.{'.'.join(rest)}@claptac.dev"


def existing_emails():
    found = set()
    for path in filter(None, run(["sudo", "find", str(USERS_DIR), "-maxdepth", "1", "-name", "*.yaml",
                                  "-print0"]).split("\0")):
        item = yaml.safe_load(run(["sudo", "cat", path])) or {}
        found.add(str(item.get("attributes", {}).get("email", "")).lower())
    return found


def check_scopes():
    problems = []
    for role, table in (("STORE_MANAGER", "SELECT id FROM shared.outlets"),
                        ("DRIVER", "SELECT vehicle_id FROM fleet.vehicles")):
        ids = set(psql(table).split())
        for name, r, scope in ROSTER:
            if r == role and scope not in ids:
                problems.append(f"{name}: {scope} does not exist")
    taken_vehicles = set(psql("SELECT vehicle_id FROM shared.driver_profiles").split())
    taken_outlets = set(psql("SELECT outlet_id FROM shared.store_manager_profiles").split())
    for name, r, scope in ROSTER:
        if r == "DRIVER" and scope in taken_vehicles:
            problems.append(f"{name}: {scope} already has a driver")
        if r == "STORE_MANAGER" and scope in taken_outlets:
            problems.append(f"{name}: {scope} already has a store manager")
        if scope is not None and not SAFE.match(scope):
            problems.append(f"{name}: unsafe scope value")
    return problems


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--apply", action="store_true", help="create the accounts (default is a dry run)")
    args = parser.parse_args()
    if os.geteuid() == 0:
        sys.exit("Run as the VM administrator without sudo; the script uses sudo where needed.")
    if not Path("docker-compose.yml").is_file():
        sys.exit("Run from the repository root (the folder with docker-compose.yml).")

    have = existing_emails()
    todo = [(n, r, s, email_for(n)) for n, r, s in ROSTER if email_for(n) not in have]
    skipped = [email_for(n) for n, _, _ in ROSTER if email_for(n) in have]
    problems = check_scopes() if todo else []

    print(f"{'DRY RUN' if not args.apply else 'APPLY'}: {len(todo)} to create, {len(skipped)} already exist")
    for name, role, scope, email in todo:
        print(f"  {role:<14} {email:<42} {scope or 'all depots'}")
    for email in skipped:
        print(f"  skip (exists) {email}")
    if problems:
        sys.exit("Stopping, nothing changed:\n  " + "\n  ".join(problems))
    if not todo or not args.apply:
        print("Nothing changed." if not args.apply else "Nothing to do.")
        return

    CREDENTIALS_DIR.mkdir(mode=0o700, parents=True, exist_ok=True)
    stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    backup = CREDENTIALS_DIR / f"users-backup-{stamp}.tar"
    run(["sudo", "tar", "-cf", str(backup), "-C", str(USERS_DIR.parent), "users"])
    run(["sudo", "chown", f"{os.getuid()}:{os.getgid()}", str(backup)])
    backup.chmod(0o600)
    list_path = CREDENTIALS_DIR / f"accounts-{stamp}.txt"
    created_files, rows, lines = [], [], []
    try:
        with tempfile.TemporaryDirectory(dir=CREDENTIALS_DIR) as staging:
            for name, role, scope, email in todo:
                identity = uuid.uuid4().hex
                user_id, subject = f"USR{identity[:12].upper()}", f"waypoint-user-{identity}"
                password = secrets.token_urlsafe(14)
                first, _, family = name.partition(" ")
                doc = {"resource_type": "user", "id": f"waypoint-person-{identity}", "type": "Person",
                       "ouId": OU_ID,
                       "attributes": {"username": email, "email": email, "sub": subject, "name": name,
                                      "given_name": first, "family_name": family},
                       "credentials": {"password": password}}
                src = Path(staging) / f"{user_id}.yaml"
                src.write_text(yaml.safe_dump(doc, sort_keys=False))
                src.chmod(0o600)
                dest = USERS_DIR / f"{user_id}.yaml"
                run(["sudo", "install", "-o", "10001", "-g", "10001", "-m", "600", str(src), str(dest)])
                created_files.append(dest)
                rows.append((user_id, subject, role, scope))
                lines.append(f"{role:<14} {email:<42} {password}   ({scope or 'all depots'})")

        sql = "BEGIN;\n"
        for user_id, subject, role, scope in rows:
            sql += f"INSERT INTO shared.users (id, identity_subject, role) VALUES ('{user_id}', '{subject}', '{role}');\n"
            table, column = PROFILE[role]
            value = "NULL" if scope is None else f"'{scope}'"
            sql += f"INSERT INTO {table} (user_id, {column}) VALUES ('{user_id}', {value});\n"
        sql += "COMMIT;\n"
        psql(sql)
    except Exception:
        for path in created_files:
            subprocess.run(["sudo", "rm", "-f", "--", str(path)])
        raise

    fd = os.open(list_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as out:
        out.write("ROLE           USERNAME                                   PASSWORD\n" + "\n".join(lines) + "\n")

    def restart_thunderid():
        run(["sudo", "docker", "restart", "waypoint-thunderid-prod"])
        status = "unknown"
        for _ in range(40):
            status = run(["sudo", "docker", "inspect", "waypoint-thunderid-prod", "--format",
                          "{{.State.Health.Status}}"]).strip()
            if status == "healthy":
                break
            time.sleep(3)
        return status

    print("Restarting ThunderID so the new accounts can sign in (about a minute)...")
    health = restart_thunderid()
    if health != "healthy":
        print(f"ThunderID is {health}. Rolling back the new accounts so existing sign-ins keep working...")
        for path in created_files:
            subprocess.run(["sudo", "rm", "-f", "--", str(path)])
        ids = ", ".join(f"'{r[0]}'" for r in rows)
        undo = "BEGIN;\n"
        for table, column in (("shared.dispatcher_profiles", None), ("shared.store_manager_profiles", None),
                              ("shared.loader_profiles", None), ("shared.driver_profiles", None)):
            undo += f"DELETE FROM {table} WHERE user_id IN ({ids});\n"
        undo += f"DELETE FROM shared.users WHERE id IN ({ids});\nCOMMIT;\n"
        psql(undo)
        list_path.unlink(missing_ok=True)
        print(f"After rollback ThunderID is {restart_thunderid()}. Nothing was kept. Backup: {backup}")
        sys.exit(1)
    print(f"ThunderID is {health}.\n")
    print(open(list_path).read())
    print(f"Saved to {list_path} (only you can read it). Put these in a password manager, then delete the file.")


if __name__ == "__main__":
    main()
