"""U0-B: native yield / co-fire and native calibration on the 694 snapshots.
Input: u0b_rows.json from cmd/t2_u0_native (-lsr top -taker raw)."""
import json,collections,datetime
from common import data, boot_ci, spearman, trigger_folds
ev=json.load(open(data('entry_study','events.json'))); byid={e['event_id']:i for i,e in enumerate(ev)}
fold=trigger_folds(ev); rows=json.load(open('u0b_rows.json')); C=collections.Counter; COST=0.0015
print('universe_in',sum(r['native_universe_in'] for r in rows),'final>=1',sum(r['native_final_fires']>0 for r in rows),'raw>=1',sum(r['native_raw_fires']>0 for r in rows))
print('co-fire final',sum(r['cofire_final'] for r in rows),'raw',sum(r['cofire_raw'] for r in rows))
print('co-fire final families',C(f for r in rows for f in set(r['cofire_final_families'] or [])).most_common(8))
ok=[]
for r in rows:
    i=byid[r['event_id']]; ts=datetime.datetime.fromisoformat(r['snap_ts']).timestamp()*1000
    bars=sorted(json.load(open(data('entry_study','klines',f'{i}.json'))))
    k=next((j for j,b in enumerate(bars) if b[0]>=ts),None)
    for s in r['final_signals'] or []:
        if k is None or s['stop']<=0 or s['tp1']<=0 or s['status']=='conflict_watch': continue
        E=bars[k][1]; L=s['direction']=='LONG'
        if (L and not s['stop']<E<s['tp1']) or ((not L) and not s['tp1']<E<s['stop']): continue
        rew=(abs(s['tp1']-E)-E*COST)/(abs(E-s['stop'])+E*COST); first='none'
        for b in bars[k:]:
            if (b[3]<=s['stop']) if L else (b[2]>=s['stop']): first='stop'; break
            if (b[2]>=s['tp1']) if L else (b[3]<=s['tp1']): first='tp1'; break
        ok.append(dict(s,first=first,R={'tp1':rew,'stop':-1.0}.get(first,0.0),fold=fold[i]))
R=[o['R'] for o in ok]; lo,hi=boot_ci(R)
print(f'calibration n={len(ok)} mean R {sum(R)/len(R):+.3f} [{lo:+.3f},{hi:+.3f}] rho(AIPriority) {spearman([o["ai_priority"] for o in ok],R):+.3f} folds',
      ' '.join('%+.3f'%spearman([o['ai_priority'] for o in ok if o['fold']==k],[o['R'] for o in ok if o['fold']==k]) for k in (1,2,3)))
for b in range(40,100,10):
    s=[o['R'] for o in ok if b<=o['ai_priority']<b+10]
    if s: l,h=boot_ci(s); print(f'  AIPriority [{b},{b+10}) n={len(s)} mean {sum(s)/len(s):+.3f} [{l:+.3f},{h:+.3f}]')
for fam,n in C(o['setup'] for o in ok).most_common():
    if n>=10: s=[o['R'] for o in ok if o['setup']==fam]; l,h=boot_ci(s); print(f'  {fam:28s} n={n} mean {sum(s)/n:+.3f} [{l:+.3f},{h:+.3f}]')
