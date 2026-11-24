#!/usr/bin/env python3
"""Write manifest.json for a Castmill OS release (ADR-0010).

Usage: release-manifest.py DIST_DIR TAG REPOSITORY

Lists the RAUC bundles in DIST_DIR (castmill-os-bundle-<machine>.raucb) by
their RAUC compatible (castmill-os-<machine>) with download URL, SHA-256 and
size. Castmill imports these manifests to offer OS updates.
"""
import datetime
import hashlib
import json
import os
import re
import sys

BUNDLE = re.compile(r"^castmill-os-bundle-(?P<machine>[a-z0-9-]+)\.raucb$")


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def manifest(dist, tag, repo, now=None):
    if not re.match(r"^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$", tag):
        raise SystemExit(f"not a release tag: {tag}")
    version = tag[1:]
    artifacts = {}
    for name in sorted(os.listdir(dist)):
        m = BUNDLE.match(name)
        if not m:
            continue
        path = os.path.join(dist, name)
        artifacts[f"castmill-os-{m['machine']}"] = {
            "url": f"https://github.com/{repo}/releases/download/{tag}/{name}",
            "sha256": sha256(path),
            "size": os.path.getsize(path),
        }
    if not artifacts:
        raise SystemExit(f"no bundles in {dist}")
    now = now or datetime.datetime.now(datetime.timezone.utc)
    return {
        "version": version,
        "prerelease": "-" in version,
        "published_at": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "notes_url": f"https://github.com/{repo}/releases/tag/{tag}",
        "security": False,
        "artifacts": artifacts,
    }


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit(__doc__)
    dist, tag, repo = sys.argv[1:]
    out = manifest(dist, tag, repo)
    with open(os.path.join(dist, "manifest.json"), "w") as f:
        json.dump(out, f, indent=2)
        f.write("\n")
    print(json.dumps(out, indent=2))
