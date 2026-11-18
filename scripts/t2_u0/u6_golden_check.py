"""U6 golden replay check: identical candidate set and tiers before and after.
Args: golden_before.json golden_after.json (cmd/t2_u0_native -golden-out, run
at the pre-U6 commit with the baseline golden_geom.go shim, and at U6).
Stamped risk values are reported separately: since the executor-gap fix the
per-setup overrides are stamped (intended), every other stamp must match."""
import json,sys,collections as C
B=json.load(open(sys.argv[1])); U=json.load(open(sys.argv[2]))
T=['key','status','tier','reason','class','admission']
S=['required_rr','required_min_stop']
cands=lambda r:r['candidates'] or []
assert len(B)==len(U), (len(B),len(U))
for cfg in sorted({r['config'] for r in U}):
    b=[r for r in B if r['config']==cfg]; u=[r for r in U if r['config']==cfg]
    tiers=lambda r:(r['event_id'],r['t2_state'],tuple(tuple(c[f] for f in T) for c in cands(r)))
    same=sum(tiers(x)==tiers(y) for x,y in zip(b,u))
    diff=C.Counter()
    for x,y in zip(b,u):
        for cb,cu in zip(cands(x),cands(y)):
            d=tuple((f,cb.get(f,0),cu.get(f,0)) for f in S+['required_max_tp'] if cb.get(f,0)!=cu.get(f,0))
            if d: diff[(cu['key'],d)]+=1
    print(f'{cfg}: rows {len(u)}, candidate set + tiers identical {same}, candidates {sum(len(cands(r)) for r in u)}')
    print('  tiers',dict(sorted(C.Counter(((c['key'].split('|')[0] or 'native'),c['tier']) for r in u for c in cands(r)).items())))
    for (k,d),n in sorted(diff.items()): print('  stamp change',k,d,'x',n)
