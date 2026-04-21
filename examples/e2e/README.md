# autopprof e2e — real Slack end-to-end test

Runs the full autopprof pipeline against a live Slack channel:

- all built-in metrics (CPU, Mem, Goroutine)
- one runtime-registered custom metric (`e2e_counter`) via `autopprof.Register`

The program generates CPU burn, memory inflation, a goroutine spike,
and bumps the custom counter past its threshold. Each metric fires a
real upload to the configured Slack channel.

## Prerequisites

- Docker (or OrbStack) — the test runs inside a cgroup-constrained
  Linux container so the cgroup queryer has CPU / memory limits to
  read.
- A Slack bot token with `files:write` on the target channel, and
  the channel ID (not the name).

## Run

From the repo root:

```bash
export SLACK_TOKEN=xoxb-…
export SLACK_CHANNEL_ID=C0123456789
./examples/e2e/run.sh
```

Optional env:

| Var | Default | Notes |
|---|---|---|
| `SLACK_APP` | `autopprof-e2e` | `"<app>"` segment in built-in filenames. |
| `E2E_DURATION` | `180s` | Total runtime. CPU metric needs ~120s of warmup before `CPUUsage()` returns non-zero (24 samples × 5s), so shorter durations won't see CPU fire. |

## Expected Slack output

Within ~150s you should see four distinct uploads land in the channel:

| File | Comment pattern |
|---|---|
| `pprof.autopprof-e2e.<host>.samples.cpu.<time>.pprof` | `:rotating_light:[CPU] usage (*X.XX%*) > threshold (*30.00%*)` |
| `pprof.autopprof-e2e.<host>.alloc_objects.alloc_space.inuse_objects.inuse_space.<time>.pprof` | `:rotating_light:[MEM] usage (*X.XX%*) > threshold (*30.00%*)` |
| `pprof.autopprof-e2e.<host>.goroutine.<time>.pprof` | `:rotating_light:[GOROUTINE] count (*N*) > threshold (*200*)` |
| `e2e_counter_<unix>.txt` | `:rotating_light:[e2e] counter=250 threshold=100` |

## Alternative: self-contained image

If you'd rather not mount the source, a multi-stage Dockerfile is
provided:

```bash
docker build -t autopprof-e2e -f examples/e2e/Dockerfile .
docker run --rm --cpus=1 -m=512m \
    -e SLACK_TOKEN -e SLACK_CHANNEL_ID -e SLACK_APP -e E2E_DURATION \
    autopprof-e2e
```
