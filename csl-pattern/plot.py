import argparse
import subprocess
from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

ENGINE_ORDER = ("Direct", "NFA", "Interpreter", "Regex", "Reuse")
ENGINE_COLORS = {
    "Direct": "#0072B2",       # Strong blue
    "NFA": "#D55E00",          # Vermillion
    "Interpreter": "#009E73",  # Bluish green
    "Regex": "#CC79A7",        # Magenta
    "Reuse": "#E69F00",        # Orange
}


def parse_benchmark_table(output):
	rows = []
	time_scale = {"ns/op": 1, "us/op": 1_000, "µs/op": 1_000, "ms/op": 1_000_000, "s/op": 1_000_000_000}
	byte_scale = {"B/op": 1, "kB/op": 1_024, "MB/op": 1_048_576}

	for line in output.splitlines():
		fields = line.split()
		if len(fields) < 7 or fields[1] not in ENGINE_ORDER:
			continue

		measurements = {}
		for index, unit in enumerate(fields[1:], start=1):
			if unit in time_scale or unit in byte_scale or unit == "allocs/op":
				try:
					measurements[unit] = float(fields[index - 1])
				except ValueError:
					continue

		try:
			time_unit = next(unit for unit in time_scale if unit in measurements)
			time_ns = measurements[time_unit] * time_scale[time_unit]
			byte_unit = next((unit for unit in byte_scale if unit in measurements), None)
			bytes_per_op = measurements[byte_unit] * byte_scale[byte_unit] if byte_unit else float(fields[5])
			allocations_per_op = measurements.get("allocs/op", float(fields[6]))
			iterations = int(fields[2])
		except (KeyError, StopIteration, ValueError):
			continue

		rows.append(
			{
				"Pattern": fields[0],
				"Engine": fields[1],
				"Iterations": iterations,
				"ns/op": time_ns,
				"B/op": bytes_per_op,
				"Allocs/op": allocations_per_op,
			}
		)

	if not rows:
		raise ValueError("No benchmark rows found; expected output from `task benchmark`.")

	return pd.DataFrame(rows)


def load_benchmark_data(input_path):
	if input_path:
		output = input_path.read_text()
	else:
		try:
			result = subprocess.run(
				["task", "benchmark"], check=True, capture_output=True, text=True
			)
		except FileNotFoundError as error:
			raise SystemExit("Task is required; install it or pass --input with saved benchmark output.") from error
		except subprocess.CalledProcessError as error:
			raise SystemExit(error.stdout + error.stderr) from error
		output = result.stdout

	try:
		return parse_benchmark_table(output)
	except ValueError as error:
		raise SystemExit(str(error)) from error


def plot_grouped_bars(ax, values, engines, colors, bar_height):
	y = np.arange(len(values.index))
	count = len(engines)
	for index, engine in enumerate(engines):
		offset = (index - (count - 1) / 2) * bar_height
		ax.barh(
			y + offset,
			values[engine].to_numpy(),
			height=bar_height,
			label=engine,
			color=colors[engine],
		)

	ax.set_yticks(y)
	ax.set_yticklabels(values.index, fontsize=8)
	ax.grid(axis="x", alpha=0.25)


def make_plots(benchmark_data, output_dir):
	latency = benchmark_data.pivot(index="Pattern", columns="Engine", values="ns/op")
	if "Regex" not in latency:
		raise SystemExit("Benchmark output does not contain Regex measurements.")

	engines = [engine for engine in ENGINE_ORDER if engine != "Regex" and engine in latency]
	ratios = pd.DataFrame(
		{engine: latency["Regex"] / latency[engine] for engine in engines}, index=latency.index
	)
	ratio_order = ratios.max(axis=1).sort_values().index
	ratios = ratios.loc[ratio_order]

	output_dir.mkdir(parents=True, exist_ok=True)
	ratio_path = output_dir / "csl_vs_regex_speed_ratio.png"
	fig, ax = plt.subplots(figsize=(12, 10))
	plot_grouped_bars(ax, ratios, engines, ENGINE_COLORS, 0.8 / len(engines))
	ax.axvline(1, color="#333333", linestyle="--", linewidth=1)
	ax.set_xlabel("Speed ratio (Regex time / engine time); above 1 is faster than Regex")
	ax.set_title("Direct, NFA, and reusable CSL performance relative to Go Regex")
	ax.legend(loc="lower right")
	fig.tight_layout()
	fig.savefig(ratio_path, dpi=180, bbox_inches="tight")
	plt.close(fig)

	latency_engines = [engine for engine in ENGINE_ORDER if engine in latency]
	latency_order = latency[latency_engines].min(axis=1).sort_values().index
	latency_values = latency.loc[latency_order, latency_engines]
	latency_path = output_dir / "csl_benchmark_latency.png"
	fig, ax = plt.subplots(figsize=(12, 10))
	plot_grouped_bars(ax, latency_values, latency_engines, ENGINE_COLORS, 0.8 / len(latency_engines))
	ax.set_xscale("log")
	ax.set_xlabel("Time (ns/op, logarithmic scale; lower is better)")
	ax.set_title("Benchmark latency by pattern and matcher")
	ax.legend(loc="lower right")
	fig.tight_layout()
	fig.savefig(latency_path, dpi=180, bbox_inches="tight")
	plt.close(fig)

	print(f"Patterns: {len(latency)}")
	for engine in engines:
		speed_ratio = (latency["Regex"] / latency[engine]).dropna()
		geometric_mean = float(np.exp(np.log(speed_ratio).mean()))
		wins = int((speed_ratio > 1).sum())
		print(f"{engine} faster than Regex: {wins}/{len(speed_ratio)}")
		print(f"{engine} geometric-mean speed ratio vs Regex: {geometric_mean:.2f}x")

	print(ratio_path)
	print(latency_path)


def main():
	parser = argparse.ArgumentParser(description="Plot CSL benchmark results.")
	parser.add_argument(
		"--input", type=Path, help="read a saved `task benchmark` table instead of running it"
	)
	parser.add_argument("--output-dir", type=Path, default=Path("."))
	args = parser.parse_args()

	make_plots(load_benchmark_data(args.input), args.output_dir)


if __name__ == "__main__":
	main()
