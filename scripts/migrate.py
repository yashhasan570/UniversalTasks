#!/usr/bin/env python3
"""Re-apply the Universal Tasks rebrand after each upstream sync (idempotent).

Protected anchors (kept intact on every run):
- LICENSE (AGPL, untouched)
- .github/ and scripts/ are excluded from the sweep entirely
"""
from pathlib import Path
import re, shutil

SKIP_EXT = {".png",".jpg",".jpeg",".ico",".webp",".woff",".woff2",".ttf",".eot",".otf",".gif",".pdf",".gz",".zip",".mp4",".webm"}
KEEP_NAMES = {"LICENSE","LICENSE.md","COPYING","COPYING.md","migrate.py"}
DELETE = ["AGENTS.md","CLAUDE.md","CHANGELOG.md","CONTRIBUTING.md","SECURITY.md","crowdin.yml",
          "renovate.json","cliff.toml","code-header-template.txt","conductor.json","publiccode.yml",
          ".github/FUNDING.yml","veans/AGENTS.md","veans/CLAUDE.md","veans/code-header-template.txt",
          "desktop/CHANGELOG.md","desktop/cliff.toml",".github/actionlint.yaml",
          ".github/scripts",".github/actions",".github/ISSUE_TEMPLATE",".github/PULL_REQUEST_TEMPLATE"]


def sweep(t):
    t = re.sub(r"Vikunja(?=[A-Z_])", "UniversalTasks", t)
    t = t.replace("VIkunja", "Universal Tasks")
    t = t.replace("VIKUNJA_", "UNIVERSALTASKS_")
    t = t.replace("VIKUNJA", "UNIVERSALTASKS")
    t = t.replace("Vikunja", "Universal Tasks")
    t = t.replace("vikunja", "universaltasks")
    return t


def main():
    for d in DELETE:
        p = Path(d)
        if p.is_dir():
            shutil.rmtree(p, ignore_errors=True)
        elif p.exists():
            p.unlink()
    for p in Path(".").rglob("*"):
        if ".git" in p.parts or not p.is_file() or p.name in KEEP_NAMES or p.suffix in SKIP_EXT:
            continue
        if str(p).startswith((".github/", "scripts/")):
            continue
        try:
            t = p.read_text()
        except Exception:
            continue
        nt = sweep(t)
        if nt != t:
            p.write_text(nt)
    for p in list(Path(".").rglob("*vikunja*")) + list(Path(".").rglob("*Vikunja*")):
        if ".git" in p.parts or p.is_dir():
            continue
        nb = p.name.replace("vikunja", "universaltasks").replace("Vikunja", "UniversalTasks")
        if nb != p.name:
            p.rename(p.with_name(nb))
    readme = Path("README.md")
    if readme.exists() and not readme.read_text().startswith("# Universal Tasks"):
        print("README drifted - rewrite needed")
    print("migrate done")


if __name__ == "__main__":
    main()
