#!/bin/sh
# merge-requirements.sh — Build-time script to generate requirements.json
# from per-package zaraos.json files.
#
# Scans a directory tree for zaraos.json files and merges them into a
# single requirements.json manifest. Used during OS image builds to
# create the baked-in default at /etc/zaraos/requirements.json.
#
# Usage:
#   ./scripts/merge-requirements.sh <packages_dir> <output_file>
#
# Example:
#   ./scripts/merge-requirements.sh ../Core/pkgs ZaraOS/overlays/etc/zaraos/requirements.json

set -e

PKGS_DIR="${1:?Usage: $0 <packages_dir> <output_file>}"
OUTPUT="${2:?Usage: $0 <packages_dir> <output_file>}"

# Find all zaraos.json files.
FILES=$(find "$PKGS_DIR" -name "zaraos.json" -maxdepth 2 | sort)

if [ -z "$FILES" ]; then
    echo "No zaraos.json files found in $PKGS_DIR"
    exit 1
fi

echo "Merging package manifests:"
for f in $FILES; do
    NAME=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['name'])" "$f" 2>/dev/null || echo "???")
    echo "  $NAME <- $f"
done

# Use Python to merge — available in both dev and CI environments.
python3 -c "
import json, sys, glob, os

pkgs_dir = sys.argv[1]
output = sys.argv[2]

packages = []
seen = set()

for path in sorted(glob.glob(os.path.join(pkgs_dir, '*/zaraos.json'))):
    with open(path) as f:
        meta = json.load(f)

    name = meta.get('name', '')
    if not name:
        print(f'  WARNING: skipping {path} — no name field', file=sys.stderr)
        continue
    if name in seen:
        print(f'  WARNING: duplicate package {name} in {path}, overwriting', file=sys.stderr)
        packages = [p for p in packages if p['name'] != name]
    seen.add(name)

    # Convert per-package meta to full package entry (add version: latest).
    pkg = dict(meta)
    pkg.setdefault('version', 'latest')
    pkg.setdefault('depends', [])
    pkg.setdefault('env', {})
    packages.append(pkg)

# Auto-calculate priority from dependency depth (same as Cortex does at runtime).
dep_depth = {}
changed = True
while changed:
    changed = False
    for pkg in packages:
        name = pkg['name']
        deps = [d for d in pkg.get('depends', []) if d in {p['name'] for p in packages}]
        if not deps:
            new_depth = 0
        else:
            max_dep = max((dep_depth.get(d, -1) for d in deps), default=-1)
            if max_dep < 0:
                continue  # deps not resolved yet
            new_depth = max_dep + 1
        if dep_depth.get(name, -1) < new_depth:
            dep_depth[name] = new_depth
            changed = True

for pkg in packages:
    pkg['priority'] = (dep_depth.get(pkg['name'], 0) + 1) * 10

manifest = {
    'version': 1,
    'packages': sorted(packages, key=lambda p: (p['priority'], p['name']))
}

os.makedirs(os.path.dirname(output) or '.', exist_ok=True)
with open(output, 'w') as f:
    json.dump(manifest, f, indent=2)
    f.write('\n')

print(f'Generated {output} with {len(packages)} packages')
" "$PKGS_DIR" "$OUTPUT"
