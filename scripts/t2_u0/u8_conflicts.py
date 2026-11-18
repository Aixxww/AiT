"""U8: conflict count re-measure (plan §4.4) and twin arms (U0-E rerun).
Inputs in the working dir: u8_conflicts.json (cmd/t2_u0_native -conflict-out),
u0b_rows.json (same run, -out), m2v2_rr15.json (t2_m2_eval -rr-min 1.5)."""
import json,collections as C
from common import data, boot_ci
ev=json.load(open(data('entry_study','events.json'))); byid={e['event_id']:i for i,e in enumerate(ev)}
rec={r['index']:r for r in json.load(open('m2v2_rr15.json'))['records']}; COST=0.0015
def t2R(i):
    e=ev[i]; r=rec[i]; E=r['e']; L=e['direction']=='LONG'
    if (L and not e['stop']<E<e['tp1']) or ((not L) and not e['tp1']<E<e['stop']): return None
    return {'tp1':(abs(e['tp1']-E)-E*COST)/(abs(E-e['stop'])+E*COST),'stop':-1.0}.get(r.get('control_first','none'),0.0)
u0b={r['event_id']:r for r in json.load(open('u0b_rows.json'))}
rows=json.load(open('u8_conflicts.json'))
print('U0-B cofire_final (pre-pass, created state):',sum(r['cofire_final'] for r in u0b.values()))
for label,pick in (('created state',lambda r:r['t2_state']!='TRIGGERED'),('TRIGGERED',lambda r:r['t2_state']=='TRIGGERED')):
    rs=[r for r in rows if pick(r)]; co=[r for r in rs if r['opposite_native']]
    print(f'\n== {label}: snapshots {len(rs)}, opposite-direction co-fires before the pass {len(co)}')
    fam=C.Counter(f for r in co for f in set(r['opposite_native']))
    print('  native families in co-fires (snapshots):',dict(fam.most_common()))
    print('  twin-pair co-fires:',sum(r['twin_opposite'] for r in co))
    out=C.Counter(x['outcome'] for r in rs for x in (r.get('resolution') or []))
    print('  final-pass resolutions:',dict(out))
    print('  unresolved after the pass (both directions survive):',sum(r['unresolved'] for r in rs))
    by=C.defaultdict(list)
    for r in co:
        o=(r.get('resolution') or [{}])[0].get('outcome','none')
        R=t2R(byid[r['event_id']])
        by[(o,'t2_kept' if r['t2_survives'] else 't2_dropped')].append(R)
    for k,v in sorted(by.items()):
        x=[y for y in v if y is not None]
        m=sum(x)/len(x) if x else float('nan'); lo,hi=boot_ci(x) if x else (None,None)
        print(f'  {k}: n={len(v)} usable={len(x)} T2 fee-net R mean {m:+.3f}'+(f' [{lo:+.3f},{hi:+.3f}]' if x else ''))
    pairs=C.Counter((r['t2_setup'],f) for r in co for f in set(r['opposite_native']))
    print('  co-fires by family pair (T2 setup, native setup):',dict(pairs.most_common()))
