#!/usr/bin/env python3
"""Entry point for the KiriVers maintainer CLI (stdlib only, no install step).

Run as `python3 dev/build/kirivers.py <command> ...`; the interpreter puts this
file's directory on sys.path, which is how `kirivers_build` resolves without a
package install or PYTHONPATH. The guard workflow executes this file straight off
the base branch, so nothing here may import third-party code.
"""

import sys

from kirivers_build import cli

if __name__ == "__main__":
    sys.exit(cli.main(sys.argv[1:]))
