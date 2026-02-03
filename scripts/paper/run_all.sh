#!/usr/bin/env bash
set -euo pipefail

# Run all experiments defined in scripts/paper/experiments.json.
#
# Assumptions:
#  - You already have dependencies running (e.g., Fabric test network if using -mode bench).
#  - You're running from anywhere inside the repo.
#
# Output:
#  - CSVs + logs in: paper_runs/<timestamp>/
#  - Analysis artifacts (tables, plots) in: paper_runs/<timestamp>/analysis/

ROOT_DIR="/home/ihors/code/military-audit-log"
CFG_FILE="$ROOT_DIR/scripts/paper/experiments.json"

if [[ ! -f "$CFG_FILE" ]]; then
  echo "ERROR: missing $CFG_FILE" >&2
  exit 1
fi

STAMP="$(date +%Y-%m-%d_%H%M%S)"
OUT_DIR_DEFAULT="$ROOT_DIR/paper_runs/$STAMP"
OUT_DIR="${1:-$OUT_DIR_DEFAULT}"

mkdir -p "$OUT_DIR"

echo "Repo: $ROOT_DIR"
echo "Config: $CFG_FILE"
echo "Output: $OUT_DIR"

# Resolve repeats from config (can override via REPEATS env var)
python3 - "$CFG_FILE" "$OUT_DIR" "${REPEATS:-}" <<'PY'
import json, os, subprocess, sys
from pathlib import Path

cfg_path = Path(sys.argv[1])
out_dir = Path(sys.argv[2])
override_repeats = sys.argv[3].strip()

cfg = json.loads(cfg_path.read_text())
go_cmd = cfg.get("go_cmd", ["go", "run", "cmd/main.go"])
common = cfg.get("common", {})
repeats = int(override_repeats) if override_repeats else int(cfg.get("repeats", 1))
experiments = cfg.get("experiments", [])

if not experiments:
    print("ERROR: no experiments in config", file=sys.stderr)
    sys.exit(2)

# Print a compact plan.
print(f"Planned experiments: {len(experiments)} configs x {repeats} repeats")

repo_root = cfg_path.resolve().parents[2]

def run_one(exp_name: str, args: list[str], overrides: dict, rep_idx: int):
    # Compose flags (common + overrides).
    count = overrides.get("count", common.get("count", 2000))
    warmup = overrides.get("warmup", common.get("warmup", 10))
    sizes = overrides.get("sizes", common.get("sizes", "4096,65536,1048576,5242880"))

    csv_name = f"{exp_name}_r{rep_idx:02d}.csv"
    csv_path = out_dir / csv_name
    log_path = out_dir / f"{exp_name}_r{rep_idx:02d}.log"

    cmd = list(go_cmd) + args + [
        "-count", str(count),
        "-warmup", str(warmup),
        "-sizes", str(sizes),
        "-out", str(csv_path),
    ]

    print("\n===", exp_name, f"(repeat {rep_idx}/{repeats})", "===")
    print("CMD:", " ".join(cmd))

    with log_path.open("w", encoding="utf-8") as lf:
        lf.write("CMD: " + " ".join(cmd) + "\n")
        lf.flush()
        p = subprocess.run(cmd, cwd=repo_root, stdout=lf, stderr=subprocess.STDOUT)
        if p.returncode != 0:
            print(f"ERROR: experiment failed: {exp_name} (see {log_path})", file=sys.stderr)
            sys.exit(p.returncode)

    if not csv_path.exists() or csv_path.stat().st_size == 0:
        print(f"ERROR: missing/empty CSV: {csv_path}", file=sys.stderr)
        sys.exit(3)

    # Light validation (fast):
    vcmd = ["python3", "scripts/paper/analyze.py", "--validate-only", "--input", str(csv_path)]
    v = subprocess.run(vcmd, cwd=repo_root)
    if v.returncode != 0:
        print(f"ERROR: validation failed for {csv_path}", file=sys.stderr)
        sys.exit(v.returncode)

for exp in experiments:
    name = exp["name"]
    args = exp.get("args", [])
    overrides = exp.get("overrides", {})

    for rep in range(1, repeats + 1):
        run_one(name, args, overrides, rep)

# Final analysis pass for the whole directory.
acmd = ["python3", "scripts/paper/analyze.py", "--input", str(out_dir), "--output", str(out_dir / "analysis")]
print("\n=== Aggregating results ===")
print("CMD:", " ".join(acmd))
subprocess.run(acmd, cwd=repo_root, check=True)
print(f"\nDone. See: {out_dir / 'analysis'}")
PY
