"""U0-A: T2 scanner score vs fee-net R on the 694 (OD-2 convention).
Input: m2v2_rr15.json (t2_m2_eval -rr-min 1.5 output) in the working dir."""
import json,collections
from common import data, boot_ci, spearman, trigger_folds
ev=json.load(open(data('entry_study','events.json')))
rec={r['index']:r for r in json.load(open('m2v2_rr15.json'))['records']}
fold=trigger_folds(ev); COST=0.0015; rows=[]
for i,e in enumerate(ev):
    r=rec[i]; E=r['e']; first=r.get('control_first','none'); L=e['direction']=='LONG'
    if (L and (E<=e['stop'] or E>=e['tp1'])) or ((not L) and (E>=e['stop'] or E<=e['tp1'])): continue
    rew=(abs(e['tp1']-E)-E*COST)/(abs(E-e['stop'])+E*COST)
    rows.append(dict(score=e['score'],setup=e['setup'],first=first,R={'tp1':rew,'stop':-1.0}.get(first,0.0),fold=fold[i]))
print('usable (non-degenerate at E):',len(rows),'of',len(ev))
def table(name,rs):
    fr=' '.join('F%d %+.3f'%(k,spearman([r['score'] for r in rs if r['fold']==k],[r['R'] for r in rs if r['fold']==k])) for k in (1,2,3) if sum(r['fold']==k for r in rs)>3)
    print(f'\n{name}: n={len(rs)} rho={spearman([r["score"] for r in rs],[r["R"] for r in rs]):+.3f}  folds: {fr}')
    b=int(min(r['score'] for r in rs)//5*5)
    while b<=max(r['score'] for r in rs):
        s=[r for r in rs if b<=r['score']<b+5]
        if s:
            R=[r['R'] for r in s]; lo,hi=boot_ci(R)
            print(f'  [{b},{b+5}) n={len(s):3d} TP1-first {100*sum(r["first"]=="tp1" for r in s)/len(s):5.1f}% mean R {sum(R)/len(R):+.3f} [{lo:+.3f},{hi:+.3f}]'+('  (n<15)' if len(s)<15 else ''))
        b+=5
table('ALL',rows)
for s,n in collections.Counter(r['setup'] for r in rows).most_common():
    if n>=30: table(s,[r for r in rows if r['setup']==s])
