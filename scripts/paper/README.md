# Paper experiment runner

This folder contains a **reproducible** workflow to:

1) run a fixed set of benchmarking experiments (baseline / direct-ledger / Merkle batching),
2) validate the CSV outputs,
3) produce paper-ready tables and plots.

## 0) Prereqs

- Go toolchain
- A working environment for your selected mode:
  - `-mode baseline`: no Fabric needed
  - `-mode bench`: Fabric test network running & chaincode deployed (your existing setup)

## 1) Run everything

From the repo root:

```bash
bash scripts/paper/run_all.sh
```

The script creates an output folder like:

```
paper_runs/2026-02-02_201500/
  csv/
  logs/
  analysis/
```

## 2) Edit the experiment grid

Modify:

- `scripts/paper/experiments.json`

Key knobs:
- `repeats`: number of independent repeats per experiment (default 3)
- `count`, `warmup`, `sizes`
- Merkle knobs per experiment: `-merkle-batch`, `-merkle-wait-ms`

## 3) Analyze an existing folder

```bash
python scripts/paper/analyze.py --input paper_runs/<folder> --output paper_runs/<folder>/analysis
```

Outputs:
- `summary_runs.csv`: one row per CSV
- `summary_grouped.csv`: grouped mean/std per experiment
- `table.tex`: LaTeX table (drop into your paper)
- `plots/*.pdf` + `plots/*.png`

## 4) Validate a single CSV

```bash
python scripts/paper/analyze.py --validate-only --input results_bench_w8_c2000.csv
```
