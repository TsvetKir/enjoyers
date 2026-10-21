"""Read-only, deliberately fictional release fixtures; no external credentials."""
import json
from pathlib import Path

DATA = Path(__file__).resolve().parent.parent / "data"

def release_section(release: str, section: str) -> dict:
    releases = json.loads((DATA / "releases.json").read_text())
    if release not in releases:
        raise ValueError("Unknown release")
    return releases[release][section]

def policy(service: str) -> dict:
    policies = json.loads((DATA / "policies.json").read_text())
    if service not in policies:
        raise ValueError("Unknown service")
    return policies[service]
