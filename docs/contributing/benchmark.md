# Measuring performance

`tools/bench` measures what the installed lerd costs on the machine it runs on, so a change can be checked against the last version before it ships. It reads the live install rather than building one, works on Linux and macOS, and keeps every run in a local history.

```bash
go run ./tools/bench            # three runs, compared with the last one from this host
go run ./tools/bench -squeeze   # also find the minimum CPU for a framework site (Linux)
go run ./tools/bench -cold      # also time a cold lerd start (restarts the stack)
```

## What a run records

Each run calibrates the machine with a fixed sha256 workload, samples idle RAM and CPU for lerd's own processes and every `lerd-` container, times `lerd sites`, then links a one-line PHP site at `~/Lerd/bench-fixture` and measures its latency and throughput. The fixture is unlinked again when the run ends, on Ctrl+C too, and the bench only ever writes into fixture folders it created itself, which carry a `.lerd-bench` marker. CPU percentages are of one core, and on Linux include the subprocesses lerd runs and reaps, such as podman and systemctl.

Results go to `~/.local/share/lerd-bench/history.jsonl`, one JSON line per run, tagged with the host, OS, CPU, RAM, lerd version and how many sites and containers were up. Runs from VMs and other machines can be appended to the same file and compared by host.

## Reading the comparison

Every metric is the median of the runs, shown with its spread. A metric is reported as `REGRESSED` only when it moved by more than 10%, more than twice the measured noise, and more than a small absolute floor (0.5% of a core, 5 MB, 1 ms), so idle jitter on a fast machine is not news. Verdicts need three or more runs on both sides, and the bench exits non-zero when anything regressed. Totals are only comparable when the same sites and containers were running; the report warns when they were not, and the per-component numbers stay valid either way.

## Minimum requirements

`-squeeze` scaffolds a Laravel app once at `~/Lerd/bench-laravel`, pins it to PHP 8.4 so the limit lands on an FPM container no worker shares, and caps nginx and that FPM container through cgroup v2 at 4, 2, 1, 0.5, 0.25, 0.1 and 0.05 CPUs. The minimum is the fewest CPUs at which p95 for four parallel requests stays under 250 ms, the recommended under 100 ms, and a pass below a failed level does not count. The original limits are written back when the run ends or is interrupted. The cap lands on the whole FPM container, so the bench refuses when other sites run on that PHP version, naming them, unless you pass `-squeeze-shared`.

CPU requirements are also given as a score, cores times the single-core calibration, so they carry over to another machine: divide the score by that machine's `calib.single_mbps` to get its cores. The RAM figure is lerd's host processes plus nginx and FPM at their peak under load, without the tray or any service.
