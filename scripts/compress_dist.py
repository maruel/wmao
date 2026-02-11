#!/usr/bin/env python3
"""Precompress frontend assets at maximum compression for embedding.

Produces .br, .zst, and .gz siblings for each file in the dist directory.
Called as a postbuild step by the frontend build script.
"""

import gzip
import subprocess
import sys
from pathlib import Path


def compress_file(path: Path) -> None:
    data = path.read_bytes()
    s = str(path)

    # Brotli (quality 11 = max).
    subprocess.run(["brotli", "--best", "--keep", s, "-o", s + ".br"], check=True)

    # Zstd (level 19).
    subprocess.run(
        ["zstd", "-19", "--quiet", "--force", "--keep", s, "-o", s + ".zst"],
        check=True,
    )

    # Gzip (level 9 = max).
    with gzip.open(s + ".gz", "wb", compresslevel=9) as f:
        f.write(data)


def main() -> None:
    dist = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("backend/frontend/dist")
    if not dist.is_dir():
        print(f"dist directory not found: {dist}", file=sys.stderr)
        sys.exit(1)

    skip = {".br", ".zst", ".gz"}
    for path in sorted(dist.rglob("*")):
        if path.is_file() and path.suffix not in skip:
            compress_file(path)


if __name__ == "__main__":
    main()
