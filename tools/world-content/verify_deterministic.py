"""Run the builder twice and verify both generated snapshots are byte-stable."""

import hashlib
import json
import subprocess
import sys

import build


def main() -> int:
    paths = [
        build.OUTPUT / "static-world-catalog.json",
        build.OUTPUT / "disabled-event-package-candidates.json",
        build.OUTPUT / "portrait-asset-audit.json",
        build.OUTPUT / "author-staging-resources.json",
        build.OUTPUT / "baseline-hash-check.json",
        build.OUTPUT / "build-report.json",
        build.ASSETS / "LUNA6_世界内容派生清单_20261015.json",
    ]
    if any(not path.is_file() for path in paths):
        raise SystemExit("Run tools/world-content/build.py once before deterministic verification.")

    def hashes():
        return {path.as_posix(): hashlib.sha256(path.read_bytes()).hexdigest() for path in paths}

    first = subprocess.run([sys.executable, "tools/world-content/build.py"], capture_output=True, text=True, check=False)
    first_hashes = hashes()
    second = subprocess.run([sys.executable, "tools/world-content/build.py"], capture_output=True, text=True, check=False)
    second_hashes = hashes()
    report = {
        "build_exit_codes": [first.returncode, second.returncode],
        "byte_identical": first_hashes == second_hashes,
        "changed_files": sorted(path for path in first_hashes if first_hashes[path] != second_hashes[path]),
        "first_builder_stdout": first.stdout.strip(),
        "second_builder_stdout": second.stdout.strip(),
    }
    print(json.dumps(report, ensure_ascii=False, sort_keys=True))
    return 0 if first.returncode == 0 and second.returncode == 0 and first_hashes == second_hashes else 1


if __name__ == "__main__":
    sys.exit(main())
