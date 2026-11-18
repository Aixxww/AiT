"""U6 golden replay check: identical candidate set and tiers before and after.
Args: golden_before.json golden_after.json (cmd/t2_u0_native -golden-out, run
at the pre-U6 commit with the baseline golden_geom.go shim, and at U6)."""
import json,sys,collections as C
B=json.load(open(sys.argv[1])); U=json.load(open(sys.argv[2]))
F=['key','status','tier','reason','class','admission','required_rr','required_min_stop']
proj=lambda r:(r['event_id'],r['t2_state'],tuple(tuple(c[f] for f in F) for c in (r['candidates'] or [])))
assert len(B)==len(U), (len(B),len(U))
for cfg in sorted({r['config'] for r in U}):
    b=[r for r in B if r['config']==cfg]; u=[r for r in U if r['config']==cfg]
    same=sum(proj(x)==proj(y) for x,y in zip(b,u))
    n=sum(len(r['candidates'] or []) for r in u)
    tp=sum(1 for r in u for c in (r['candidates'] or []) if c.get('required_max_tp'))
    print(f'{cfg}: rows {len(u)} identical {same}, candidates {n}, max_tp stamped {tp}')
    print('  tiers',dict(sorted(C.Counter(((c['key'].split('|')[0] or 'native'),c['tier']) for r in u for c in (r['candidates'] or [])).items())))
