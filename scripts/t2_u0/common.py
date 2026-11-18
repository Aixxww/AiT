"""Shared helpers for the U0 evidence scripts (redesign plan U0).

Paths: U0_DATA is the private data root (entry_study/, lookback/,
m3_snapshots/, measure/, live_db/). Scripts read and write their working
files in the current directory. No data is ever committed.
"""
import os
import random

DATA = os.environ.get("U0_DATA", "")


def data(*parts):
    if not DATA:
        raise SystemExit("set U0_DATA to the private ait-t2-data checkout")
    return os.path.join(DATA, *parts)


def pct(xs, p):
    """Linear-interpolation percentile (numpy default)."""
    s = sorted(xs)
    k = (len(s) - 1) * p / 100
    f = int(k)
    c = min(f + 1, len(s) - 1)
    return s[f] + (s[c] - s[f]) * (k - f)


def boot_ci(xs, n=5000, lo=5, hi=95, seed=20261118):
    """90% bootstrap CI of the mean."""
    if not xs:
        return (None, None)
    rnd = random.Random(seed)
    m = len(xs)
    means = sorted(sum(xs[rnd.randrange(m)] for _ in range(m)) / m for _ in range(n))
    return (means[int(n * lo / 100)], means[int(n * hi / 100) - 1])


def spearman(xs, ys):
    def rank(v):
        o = sorted(range(len(v)), key=lambda k: v[k])
        r = [0.0] * len(v)
        k = 0
        while k < len(o):
            j = k
            while j + 1 < len(o) and v[o[j + 1]] == v[o[k]]:
                j += 1
            for t in range(k, j + 1):
                r[o[t]] = (k + j) / 2
            k = j + 1
        return r

    a, b = rank(xs), rank(ys)
    n = len(a)
    ma, mb = sum(a) / n, sum(b) / n
    cov = sum((x - ma) * (y - mb) for x, y in zip(a, b))
    va = sum((x - ma) ** 2 for x in a)
    vb = sum((y - mb) ** 2 for y in b)
    return cov / (va * vb) ** 0.5 if va > 0 and vb > 0 else float("nan")


def trigger_folds(events):
    """OD-2 folds: trigger-time tertiles over all events (index -> 1..3)."""
    order = sorted(range(len(events)), key=lambda i: events[i]["trig_ms"])
    return {i: 1 + k * 3 // len(events) for k, i in enumerate(order)}
