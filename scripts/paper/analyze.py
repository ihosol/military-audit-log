#!/usr/bin/env python3
"""Analyze experiment CSV outputs and generate paper-ready tables + plots.

Usage examples:
  # Validate one CSV (fast sanity checks)
  python scripts/paper/analyze.py --validate-only --input results_bench_w8_c2000.csv

  # Analyze a directory with many CSVs
  python scripts/paper/analyze.py --input paper_runs/2026-02-02_201500 --output paper_runs/2026-02-02_201500/analysis

Outputs (in --output):
  - summary_runs.csv            (one row per run file)
  - summary_grouped.csv         (mean/std over repeats per experiment)
  - table_grouped.tex           (LaTeX tabular for the paper)
  - plots/*.pdf + plots/*.png

Notes:
  - This script assumes the CSV format produced by cmd/main.go (with timestamp fields).
  - Blob-hash verification is attempted only for local file paths that exist.
"""

from __future__ import annotations

import argparse
import glob
import hashlib
import math
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Tuple

import pandas as pd
import matplotlib.pyplot as plt

# ----------- config -----------

DURATION_COLS = [
    "total_sec",
    "hash_sec",
    "storage_sec",
    "db_sec",
    "ledger_sec",
    "merkle_wait_sec",
    "merkle_build_sec",
    "merkle_ledger_sec",
]

TS_COLS = [
    "req_start_unix_ns",
    "req_end_unix_ns",
]

DEFAULT_EPS_SEC = 0.050  # stage-sum vs total tolerance; allows clock jitter / rounding


def _ensure_cols(df: pd.DataFrame, cols: Iterable[str], default=0) -> pd.DataFrame:
    for c in cols:
        if c not in df.columns:
            df[c] = default
    return df


def _coerce_numeric(df: pd.DataFrame, cols: Iterable[str]) -> pd.DataFrame:
    for c in cols:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce").fillna(0)
    return df


def _coerce_str(df: pd.DataFrame, cols: Iterable[str]) -> pd.DataFrame:
    for c in cols:
        if c in df.columns:
            # IMPORTANT: empty CSV cells are parsed as NaN.
            # If we call astype(str) first, NaN becomes the literal string "nan",
            # which breaks validation (e.g., baseline appears to have tx_id/merkle_root).
            s = df[c].fillna("").astype(str)
            # Normalize common "missing" string sentinels.
            s = s.replace({"nan": "", "NaN": "", "None": "", "<nil>": "", "null": ""})
            df[c] = s
    return df


def scol(df: pd.DataFrame, col: str, default: str = "") -> pd.Series:
    """Return a *string* Series for a DataFrame column, safe for length checks.

    Pandas may parse empty strings as NaN; converting NaN -> 'nan' breaks .str / len checks.
    This helper normalizes missing values to empty strings and strips common NA spellings.
    """
    if col not in df.columns:
        return pd.Series([default] * len(df), index=df.index)
    s = df[col]
    if not isinstance(s, pd.Series):
        s = pd.Series(s, index=df.index)
    s = s.fillna("")
    s = s.astype(str)
    s = s.replace({"nan": "", "NaN": "", "None": "", "<nil>": "", "null": "", "<NA>": ""})
    return s



def _quantile(series: pd.Series, q: float) -> float:
    if len(series) == 0:
        return float("nan")
    return float(series.quantile(q, interpolation="linear"))


def _sha256_file(path: Path, chunk_size: int = 1024 * 1024) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        while True:
            b = f.read(chunk_size)
            if not b:
                break
            h.update(b)
    return h.hexdigest()


@dataclass
class ValidationResult:
    ok: bool
    messages: List[str]


def load_csv(path: Path) -> pd.DataFrame:
    df = pd.read_csv(path)
    df = _ensure_cols(df, DURATION_COLS, default=0)
    df = _ensure_cols(df, TS_COLS, default=0)
    df = _ensure_cols(df, ["mode", "workers", "is_warmup", "status", "error", "tx_id", "merkle_root", "doc_hash_hex", "storage_path"], default="")

    # Coerce types
    df = _coerce_numeric(df, DURATION_COLS)
    df = _coerce_numeric(df, TS_COLS)
    df["workers"] = pd.to_numeric(df["workers"], errors="coerce").fillna(0).astype(int)

    # is_warmup can be bool or int; normalize to 0/1
    if df["is_warmup"].dtype == bool:
        df["is_warmup"] = df["is_warmup"].astype(int)
    else:
        df["is_warmup"] = pd.to_numeric(df["is_warmup"], errors="coerce").fillna(0).astype(int)

    df = _coerce_str(df, ["mode", "status", "error", "tx_id", "merkle_root", "doc_hash_hex", "storage_path"])

    return df


def validate_csv(path: Path, *, eps_sec: float = DEFAULT_EPS_SEC, verify_blob_sample: int = 0) -> ValidationResult:
    msgs: List[str] = []
    ok = True

    try:
        df = load_csv(path)
    except Exception as e:
        return ValidationResult(False, [f"failed to parse CSV: {e}"])

    rows_total = len(df)
    rows_warmup = int((df["is_warmup"] == 1).sum())
    rows_main = rows_total - rows_warmup

    if rows_total <= 0:
        return ValidationResult(False, ["empty CSV"])

    msgs.append(f"rows_total={rows_total} warmup={rows_warmup} main={rows_main}")

    # Negative durations
    neg_cols = {}
    for c in DURATION_COLS:
        neg = int((df[c] < -1e-12).sum())
        if neg:
            neg_cols[c] = neg
    if neg_cols:
        ok = False
        msgs.append(f"negative_durations={neg_cols}")

    # Stage sum vs total (only for main rows)
    dfm = df[df["is_warmup"] == 0]
    stage_sum = dfm["hash_sec"] + dfm["storage_sec"] + dfm["db_sec"] + dfm["ledger_sec"] + dfm["merkle_wait_sec"]
    diff = stage_sum - dfm["total_sec"]
    max_abs = float(diff.abs().max()) if len(diff) else 0.0
    if max_abs > eps_sec:
        ok = False
        msgs.append(f"stage_sum_minus_total_max_abs={max_abs:.6f}s (eps={eps_sec:.3f}s)")

    # Sanity: if baseline, expect ledger/merkle fields empty/zero
    mode = str(df["mode"].iloc[0]) if rows_total else ""
    mr_nonempty_any = bool((scol(df, "merkle_root").map(len) > 0).any())
    mbs = df.get("merkle_batch_size", pd.Series([0] * len(df), index=df.index))
    mbs_num = pd.to_numeric(mbs, errors="coerce").fillna(0)
    has_merkle = bool(mr_nonempty_any or (mbs_num > 0).any())
    tx_nonempty = int((scol(df, "tx_id").map(len) > 0).sum())
    merkle_nonempty = int((scol(df, "merkle_root").map(len) > 0).sum())
    if mode == "baseline":
        if tx_nonempty != 0:
            ok = False
            msgs.append(f"baseline_expected_no_tx_id_but_found={tx_nonempty}")
        if merkle_nonempty != 0:
            ok = False
            msgs.append(f"baseline_expected_no_merkle_root_but_found={merkle_nonempty}")

    # Status / errors
    bad_status = int((scol(dfm, "status").map(lambda x: x.lower()) != "ok").sum())
    if bad_status:
        ok = False
        msgs.append(f"non_ok_status_main_rows={bad_status}")
        # show a couple
        bad_mask = (scol(dfm, "status").map(lambda x: x.lower()) != "ok")
        msgs.append("example_errors=" + "; ".join(scol(dfm[bad_mask], "error").head(3).tolist()))
    # Optional: verify local blob hashes
    if verify_blob_sample > 0:
        verified_ok = 0
        verified_bad = 0
        verified_skipped = 0
        sample = dfm.head(verify_blob_sample)
        for _, r in sample.iterrows():
            sp = str(r.get("storage_path", ""))
            expected = str(r.get("doc_hash_hex", ""))
            if not sp or not expected:
                verified_skipped += 1
                continue
            p = Path(sp)
            # Many runs store relative paths; try repo-root relative
            if not p.exists() and not p.is_absolute():
                p2 = path.parent / p
                if p2.exists():
                    p = p2
            if not p.exists() or not p.is_file():
                verified_skipped += 1
                continue
            got = _sha256_file(p)
            if got.lower() == expected.lower():
                verified_ok += 1
            else:
                verified_bad += 1
        if verified_bad:
            ok = False
        msgs.append(f"blob_hash_verify_ok={verified_ok} bad={verified_bad} skipped={verified_skipped} (sample={verify_blob_sample})")

    return ValidationResult(ok, msgs)


def compute_run_metrics(csv_path: Path, *, eps_sec: float = DEFAULT_EPS_SEC, verify_blob_sample: int = 0) -> Dict[str, object]:
    df = load_csv(csv_path)

    rows_total = len(df)
    rows_warmup = int((df["is_warmup"] == 1).sum())
    dfm = df[df["is_warmup"] == 0]
    rows_main = len(dfm)

    # Determine merkle mode by presence of merkle_root
    merkle_rows = dfm[(scol(dfm, "merkle_root").map(len) > 0)]
    is_merkle = len(merkle_rows) > 0

    # Compute throughput from request timestamps (more robust with concurrency)
    tps = float("nan")
    wall = float("nan")
    if rows_main > 0 and (dfm["req_start_unix_ns"].max() > 0) and (dfm["req_end_unix_ns"].max() > 0):
        t0 = float(dfm["req_start_unix_ns"].min())
        t1 = float(dfm["req_end_unix_ns"].max())
        wall = max(0.0, (t1 - t0) / 1e9)
        if wall > 0:
            tps = rows_main / wall

    def lat_stats(col: str) -> Tuple[float, float, float, float]:
        s = dfm[col]
        return (
            float(s.mean()) if len(s) else float("nan"),
            _quantile(s, 0.50),
            _quantile(s, 0.95),
            _quantile(s, 0.99),
        )

    total_mean, total_p50, total_p95, total_p99 = lat_stats("total_sec")
    ledger_mean, ledger_p50, ledger_p95, ledger_p99 = lat_stats("ledger_sec")
    merkle_wait_mean, merkle_wait_p50, merkle_wait_p95, merkle_wait_p99 = lat_stats("merkle_wait_sec")

    # Ledger transactions
    tx_unique = scol(dfm, "tx_id")[scol(dfm, "tx_id").map(len) > 0].nunique(dropna=True)
    merkle_root_unique = scol(dfm, "merkle_root")[scol(dfm, "merkle_root").map(len) > 0].nunique(dropna=True)
    ledger_tx = int(merkle_root_unique if is_merkle else tx_unique)
    ledger_tx_per_req = float(ledger_tx / rows_main) if rows_main else float("nan")

    # Merkle batch parameters (best-effort)
    merkle_batch_size = int(pd.to_numeric(df.get("merkle_batch_size", pd.Series([0])), errors="coerce").fillna(0).max())

    # Validation summary
    v = validate_csv(csv_path, eps_sec=eps_sec, verify_blob_sample=verify_blob_sample)

    # Extract "experiment id" from filename
    base = csv_path.name
    exp_id = re.sub(r"\.csv$", "", base)
    # remove trailing repeat tag: _r01, _r1, -r01
    exp_group = re.sub(r"([_-]r\d+)$", "", exp_id)

    out: Dict[str, object] = {
        "file": str(csv_path),
        "experiment_id": exp_id,
        "experiment_group": exp_group,
        "mode": str(df["mode"].iloc[0]) if rows_total else "",
        "workers": int(df["workers"].iloc[0]) if rows_total else 0,
        "is_merkle": bool(is_merkle),
        "merkle_batch_size": merkle_batch_size,
        "rows_total": rows_total,
        "rows_warmup": rows_warmup,
        "rows_main": rows_main,
        "wall_sec": wall,
        "tps": tps,
        "total_mean_sec": total_mean,
        "total_p50_sec": total_p50,
        "total_p95_sec": total_p95,
        "total_p99_sec": total_p99,
        "ledger_mean_sec": ledger_mean,
        "ledger_p95_sec": ledger_p95,
        "merkle_wait_mean_sec": merkle_wait_mean,
        "merkle_wait_p95_sec": merkle_wait_p95,
        "ledger_tx": ledger_tx,
        "ledger_tx_per_req": ledger_tx_per_req,
        "validation_ok": v.ok,
        "validation_messages": " | ".join(v.messages),
    }

    return out


def analyze_dir(input_path: Path, output_dir: Path, *, verify_blob_sample: int = 0, eps_sec: float = DEFAULT_EPS_SEC) -> int:
    output_dir.mkdir(parents=True, exist_ok=True)
    (output_dir / "plots").mkdir(parents=True, exist_ok=True)

    csv_files: List[Path] = []
    if input_path.is_file() and input_path.suffix.lower() == ".csv":
        csv_files = [input_path]
    else:
        csv_files = [Path(p) for p in glob.glob(str(input_path / "**" / "*.csv"), recursive=True)]

    # Ignore outputs we generate ourselves
    csv_files = [p for p in csv_files if "summary_" not in p.name and "_analysis" not in str(p)]
    csv_files.sort()

    if not csv_files:
        print(f"No CSV files found under: {input_path}", file=sys.stderr)
        return 2

    rows: List[Dict[str, object]] = []
    failures = 0
    for p in csv_files:
        m = compute_run_metrics(p, verify_blob_sample=verify_blob_sample, eps_sec=eps_sec)
        rows.append(m)
        if not bool(m["validation_ok"]):
            failures += 1

    runs_df = pd.DataFrame(rows)
    runs_df.to_csv(output_dir / "summary_runs.csv", index=False)

    # Group repeats
    group_cols = ["experiment_group", "mode", "workers", "is_merkle", "merkle_batch_size"]
    agg_df = (
        runs_df
        .groupby(group_cols, dropna=False)
        .agg(
            tps_mean=("tps", "mean"),
            tps_std=("tps", "std"),
            total_p95_mean=("total_p95_sec", "mean"),
            total_p95_std=("total_p95_sec", "std"),
            total_p99_mean=("total_p99_sec", "mean"),
            total_p99_std=("total_p99_sec", "std"),
            ledger_tx_per_req_mean=("ledger_tx_per_req", "mean"),
            ledger_tx_per_req_std=("ledger_tx_per_req", "std"),
            merkle_wait_p95_mean=("merkle_wait_p95_sec", "mean"),
            merkle_wait_p95_std=("merkle_wait_p95_sec", "std"),
            n_runs=("file", "count"),
            validation_ok_all=("validation_ok", "all"),
        )
        .reset_index()
        .sort_values(["mode", "is_merkle", "merkle_batch_size", "workers", "experiment_group"])
    )

    agg_df.to_csv(output_dir / "summary_grouped.csv", index=False)

    # LaTeX table (grouped)
    tex_path = output_dir / "table_grouped.tex"
    with tex_path.open("w", encoding="utf-8") as f:
        f.write("% Auto-generated by scripts/paper/analyze.py\n")
        f.write("% Columns: experiment, mode, workers, merkle, batch, TPS, p95 latency, ledger tx/req\n")
        f.write("\\begin{tabular}{l l r l r r r}\n")
        f.write("\\toprule\n")
        f.write("Experiment & Mode & W & Merkle & Batch & TPS (mean$\\pm$sd) & p95 latency [s] \\\\ \\\n")
        f.write("\\midrule\n")
        for _, r in agg_df.iterrows():
            exp = str(r["experiment_group"]).replace("_", "\\_")
            mode = str(r["mode"]).replace("_", "\\_")
            w = int(r["workers"])
            merkle = "yes" if bool(r["is_merkle"]) else "no"
            b = int(r["merkle_batch_size"]) if bool(r["is_merkle"]) else 0
            tps = float(r["tps_mean"]) if not math.isnan(float(r["tps_mean"])) else 0.0
            tps_sd = float(r["tps_std"]) if not math.isnan(float(r["tps_std"])) else 0.0
            p95 = float(r["total_p95_mean"]) if not math.isnan(float(r["total_p95_mean"])) else 0.0
            p95_sd = float(r["total_p95_std"]) if not math.isnan(float(r["total_p95_std"])) else 0.0

            f.write(f"{exp} & {mode} & {w} & {merkle} & {b} & {tps:.2f}$\\pm${tps_sd:.2f} & {p95:.3f}$\\pm${p95_sd:.3f} \\\\n")
        f.write("\\bottomrule\n")
        f.write("\\end{tabular}\n")

    # ---- Plots ----
    # Helper label
    def label_row(r: pd.Series) -> str:
        if bool(r["is_merkle"]):
            return f"merkle(b={int(r['merkle_batch_size'])})"
        return str(r["mode"])

    plot_df = agg_df.copy()
    plot_df["label"] = plot_df.apply(label_row, axis=1)

    # 1) TPS vs workers
    _plot_bar_by_workers(
        plot_df,
        y_col="tps_mean",
        yerr_col="tps_std",
        title="Throughput (TPS) by workers",
        ylabel="TPS (req/sec)",
        out_prefix=output_dir / "plots" / "tps_by_workers",
    )

    # 2) p95 latency vs workers
    _plot_bar_by_workers(
        plot_df,
        y_col="total_p95_mean",
        yerr_col="total_p95_std",
        title="p95 end-to-end latency by workers",
        ylabel="p95 latency (sec)",
        out_prefix=output_dir / "plots" / "latency_p95_by_workers",
    )

    # 3) Ledger tx per request (shows batching effectiveness)
    _plot_bar_by_workers(
        plot_df,
        y_col="ledger_tx_per_req_mean",
        yerr_col="ledger_tx_per_req_std",
        title="Ledger transactions per request",
        ylabel="ledger tx / request",
        out_prefix=output_dir / "plots" / "ledger_tx_per_req_by_workers",
        ylim_floor=0.0,
    )

    # 4) Merkle p95 wait (only merkle rows)
    merkle_only = plot_df[plot_df["is_merkle"] == True].copy()
    if len(merkle_only):
        _plot_bar_by_workers(
            merkle_only,
            y_col="merkle_wait_p95_mean",
            yerr_col="merkle_wait_p95_std",
            title="Merkle p95 wait time by workers",
            ylabel="p95 merkle wait (sec)",
            out_prefix=output_dir / "plots" / "merkle_wait_p95_by_workers",
        )

    # Also print a short console summary
    print("\n=== Summary (grouped) ===")
    print(agg_df[["experiment_group", "mode", "workers", "is_merkle", "merkle_batch_size", "tps_mean", "total_p95_mean", "ledger_tx_per_req_mean", "validation_ok_all"]].to_string(index=False))

    if failures:
        print(f"\nWARNING: {failures} run file(s) failed validation. See summary_runs.csv for details.", file=sys.stderr)
        return 1
    return 0


def _plot_bar_by_workers(
    df: pd.DataFrame,
    *,
    y_col: str,
    yerr_col: str,
    title: str,
    ylabel: str,
    out_prefix: Path,
    ylim_floor: Optional[float] = None,
):
    # Pivot to (workers x label)
    workers_sorted = sorted(df["workers"].unique().tolist())
    labels_sorted = sorted(df["label"].unique().tolist())

    # Build arrays
    import numpy as np

    y = np.zeros((len(labels_sorted), len(workers_sorted)))
    yerr = np.zeros_like(y)

    for i, lab in enumerate(labels_sorted):
        for j, w in enumerate(workers_sorted):
            row = df[(df["label"] == lab) & (df["workers"] == w)]
            if len(row) == 0:
                y[i, j] = np.nan
                yerr[i, j] = 0.0
            else:
                y[i, j] = float(row[y_col].iloc[0])
                yerr[i, j] = float(row[yerr_col].iloc[0]) if not pd.isna(row[yerr_col].iloc[0]) else 0.0

    x = np.arange(len(workers_sorted))
    width = 0.8 / max(1, len(labels_sorted))

    fig, ax = plt.subplots(figsize=(8.2, 4.2))
    for i, lab in enumerate(labels_sorted):
        ax.bar(
            x + (i - (len(labels_sorted) - 1) / 2) * width,
            y[i],
            width,
            yerr=yerr[i],
            capsize=3,
            label=lab,
        )

    ax.set_title(title)
    ax.set_xlabel("workers")
    ax.set_ylabel(ylabel)
    ax.set_xticks(x)
    ax.set_xticklabels([str(w) for w in workers_sorted])
    if ylim_floor is not None:
        lo, hi = ax.get_ylim()
        ax.set_ylim(bottom=ylim_floor, top=hi)
    ax.legend(loc="best", frameon=False)
    ax.grid(axis="y", linestyle=":", linewidth=0.8)
    fig.tight_layout()

    fig.savefig(str(out_prefix) + ".pdf")
    fig.savefig(str(out_prefix) + ".png", dpi=220)
    plt.close(fig)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", required=True, help="CSV file or directory with CSVs")
    ap.add_argument("--output", default="analysis", help="output directory (when analyzing a directory)")
    ap.add_argument("--validate-only", action="store_true", help="only validate the input CSV file")
    ap.add_argument("--verify-blob-sample", type=int, default=0, help="verify SHA256 of local blob files for first N main rows (best-effort)")
    ap.add_argument("--eps-sec", type=float, default=DEFAULT_EPS_SEC, help="tolerance for stage_sum vs total")

    args = ap.parse_args()
    inp = Path(args.input)

    if args.validate_only:
        if not inp.is_file():
            print("--validate-only expects --input to be a CSV file", file=sys.stderr)
            return 2
        v = validate_csv(inp, eps_sec=args.eps_sec, verify_blob_sample=args.verify_blob_sample)
        print(f"VALID={v.ok}")
        for m in v.messages:
            print(m)
        return 0 if v.ok else 1

    out = Path(args.output)
    return analyze_dir(inp, out, verify_blob_sample=args.verify_blob_sample, eps_sec=args.eps_sec)


if __name__ == "__main__":
    raise SystemExit(main())
