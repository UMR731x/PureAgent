"""Select the latest version notes and publish only when that document changes."""

import os
from pathlib import Path
import re
import subprocess


def version_numbers(path):
    match = re.fullmatch(r"v(\d+)\.(\d+)\.(\d+)\.md", path.name)
    return tuple(map(int, match.groups())) if match else ()


notes = max(
    (path for path in Path(".github/releases").glob("v*.md") if version_numbers(path)),
    key=version_numbers,
)
publish = False
if os.environ["GITHUB_REF"] == "refs/heads/main":
    if os.environ["GITHUB_EVENT_NAME"] == "workflow_dispatch":
        publish = True
    elif os.environ["GITHUB_EVENT_NAME"] == "push":
        before = os.environ["PUSH_BEFORE"]
        if before.strip("0"):
            changed = subprocess.check_output(
                ["git", "diff", "--name-only", before, "HEAD", "--", ".github/releases"],
                text=True,
            ).splitlines()
            publish = notes.as_posix() in changed
        else:
            publish = True

with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
    output.write(f"publish={str(publish).lower()}\n")
    output.write(f"version={notes.stem if publish else 'dev'}\n")
    output.write(f"notes={notes.as_posix()}\n")
