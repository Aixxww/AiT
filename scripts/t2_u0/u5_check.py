"""U5 acceptance: tags on every replayed record, and tracker fee-net R vs the
U0 walk (U0-A control arm, OD-2 convention) on the 694.
Inputs in the working dir: m2v2_rr15.json (t2_m2_eval -rr-min 1.5) and one or
more cmd/t2_u5_tags outputs given as arguments."""
import json,collections as C,sys
from common import data
ev=json.load(open(data('entry_study','events.json')))
rec={r['index']:r for r in json.load(open('m2v2_rr15.json'))['records']}
COST=0.0015; ref={}
for i,e in enumerate(ev):
    r=rec[i]; E=r['e']; first=r.get('control_first','none'); L=e['direction']=='LONG'
    if (L and (E<=e['stop'] or E>=e['tp1'])) or ((not L) and (E>=e['stop'] or E<=e['tp1'])):
        ref[e['event_id']]=dict(E=E,first='degenerate',R=None); continue
    rew=(abs(e['tp1']-E)-E*COST)/(abs(E-e['stop'])+E*COST)
    ref[e['event_id']]=dict(E=E,first=first,R={'tp1':rew,'stop':-1.0}.get(first,0.0))
for fn in sys.argv[1:]:
    rows=json.load(open(fn)); print('==',fn)
    print('recalled',sum(r['recalled'] for r in rows),'of',len(rows))
    st=[s for r in rows for s in r['states']]
    print('records',len(st),'with missing tags',sum(bool(s.get('missing_tags')) for s in st),dict(C.Counter(m for s in st for m in s.get('missing_tags') or [])))
    print('tracker fee-net outcome',dict(C.Counter(r['fee_net_outcome'] for r in rows)))
    print('entry E identical',sum(abs(r['fee_net_entry']-ref[r['event_id']]['E'])<1e-12 for r in rows),'/',len(rows))
    usable=[r for r in rows if ref[r['event_id']]['R'] is not None]
    eq=[r for r in usable if r['fee_net_r'] is not None and r['fee_net_r']==ref[r['event_id']]['R']]
    print('U0-A usable',len(usable),'tracker R identical',len(eq))
    print('mismatch (U0 first, tracker outcome)',dict(C.Counter((ref[r['event_id']]['first'],r['fee_net_outcome']) for r in usable if r not in eq)))
    deg=[r for r in rows if ref[r['event_id']]['R'] is None]
    print('U0 degenerate',len(deg),'tracker',dict(C.Counter(r['fee_net_outcome'] for r in deg)))
    print('mean R on usable: tracker %+.4f U0 %+.4f'%(sum(r['fee_net_r'] or 0 for r in usable)/len(usable),sum(ref[r['event_id']]['R'] for r in usable)/len(usable)))
