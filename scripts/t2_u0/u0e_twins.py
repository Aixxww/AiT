"""U0-E: twin-pair arms on the 694 snapshots. Inputs: u0b_rows.json, m2v2_rr15.json."""
import json,collections
from common import data, boot_ci
ev=json.load(open(data('entry_study','events.json'))); byid={e['event_id']:i for i,e in enumerate(ev)}
rec={r['index']:r for r in json.load(open('m2v2_rr15.json'))['records']}; COST=0.0015
def t2R(i):
    e=ev[i]; r=rec[i]; E=r['e']; L=e['direction']=='LONG'
    if (L and not e['stop']<E<e['tp1']) or ((not L) and not e['tp1']<E<e['stop']): return None
    return {'tp1':(abs(e['tp1']-E)-E*COST)/(abs(E-e['stop'])+E*COST),'stop':-1.0}.get(r.get('control_first','none'),0.0)
pairs={'A':('bb_oi_accumulation','accumulation_breakout_long'),'B':('momentum_continuation_','leader_momentum_long'),
       'C':('compression_oi_watch','volatility_squeeze_breakout'),'D':('confirmed_breakout','trend_breakout_long')}
rows=json.load(open('u0b_rows.json'))
for name,(t2p,nat) in pairs.items():
    arms=collections.defaultdict(list)
    for r in rows:
        i=byid[r['event_id']]; is_t2=ev[i]['setup'].startswith(t2p); nf=[s for s in r['final_signals'] or [] if s['setup']==nat]
        if is_t2 and nf: arm='both_same' if nf[0]['direction']==ev[i]['direction'] else 'both_opposite'
        elif is_t2: arm='only_t2'
        elif nf: arm='only_native'
        else: continue
        arms[arm].append(t2R(i) if arm!='only_native' else None)
    print(name,{a:len(v) for a,v in arms.items()})
