#!/usr/bin/env python3
"""Run ESB unit tests whose source files live outside the ESB project."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[2]
ESB_ROOT = ROOT / "predictor" / "esb"
TEST_ROOT = ROOT / "predictor" / "tests" / "unit" / "esb"


def main() -> int:
    replacements = {
        str(ESB_ROOT / test_file.relative_to(TEST_ROOT)): str(test_file)
        for test_file in sorted(TEST_ROOT.rglob("*_test.go"))
    }
    if not replacements:
        print(f"no ESB unit tests found under {TEST_ROOT}", file=sys.stderr)
        return 2

    with tempfile.TemporaryDirectory(prefix="predictor-go-overlay-") as temp_dir:
        overlay = Path(temp_dir) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}), encoding="utf-8")
        command = [
            os.environ.get("GO", "go"),
            "test",
            "-overlay",
            str(overlay),
            *sys.argv[1:],
        ]
        return subprocess.run(command, cwd=ESB_ROOT, check=False).returncode


if __name__ == "__main__":
    raise SystemExit(main())
