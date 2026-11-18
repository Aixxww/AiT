import json,datetime,collections
from common import boot_ci
import os
D=os.environ['U0_DATA']
ev=json.load(open(D+'/entry_study/events.json')); byid={e['event_id']:i for i,e in enumerate(ev)}
COST=0.0015; DELAYS=[0,1,5,15]
def outcome(bars,k,L,stop,tp1):
    """market entry at close of bar k; returns (status,R)"""
    if k>=len(bars): return ('nodata',None)
    # run-away / invalidated before entry: TP1 or stop touched between ref bar and entry
    E=bars[k][4]
    if (L and E>=tp1) or ((not L) and E<=tp1): return ('runaway',None)
    if (L and E<=stop) or ((not L) and E>=stop): return ('invalidated',None)
    rew=(abs(tp1-E)-E*COST)/(abs(E-stop)+E*COST)
    for b in bars[k+1:]:
        if (b[3]<=stop) if L else (b[2]>=stop): return ('stop',-1.0)
        if (b[2]>=tp1) if L else (b[3]<=tp1): return ('tp1',rew)
    return ('none',0.0)
def run(items):
    # items: (family, bars, k0, L, stop, tp1)
    res=collections.defaultdict(lambda: collections.defaultdict(list)); ra=collections.defaultdict(lambda: collections.Counter())
    for fam,bars,k0,L,stop,tp1 in items:
        base=outcome(bars,k0,L,stop,tp1)
        for dl in DELAYS:
            st,R=outcome(bars,k0+dl,L,stop,tp1)
            ra[(fam,dl)][st]+=1
            if base[1] is not None and R is not None: res[fam][dl].append(R-base[1])
            if base[1] is not None and R is None and st=='runaway': res[fam][dl].append(None)
    return res,ra
def report(name,items,minn=15):
    res,ra=run(items)
    fams=collections.Counter(i[0] for i in items)
    print(f'\n=== {name} ===  (delay cost = R(entry Δ min later) − R(entry at signal bar close), paired, same plan stop/TP1; run-away = TP1 already passed at the delayed entry)')
    for fam,n in [('ALL',len(items))]+fams.most_common():
        if n<minn: continue
        cells=[]
        for dl in DELAYS[1:]:
            pool=[x for f in (fams if fam=='ALL' else [fam]) for x in res[f][dl]]
            dr=[x for x in pool if x is not None]
            c=collections.Counter(); 
            for f in (fams if fam=='ALL' else [fam]): c+=ra[(f,dl)]
            tot=sum(c.values()); lo,hi=boot_ci(dr) if dr else (0,0)
            cells.append(f"Δ{dl:>2}m: {sum(dr)/len(dr) if dr else 0:+.3f} [{lo:+.3f},{hi:+.3f}] runaway {100*c['runaway']/tot:4.1f}%")
        c0=collections.Counter()
        for f in (fams if fam=='ALL' else [fam]): c0+=ra[(f,0)]
        print(f"  {fam:32s} n={n:3d} runaway@0 {100*c0['runaway']/sum(c0.values()):4.1f}% | "+' | '.join(cells))
# T2: signal bar = the 1m bar containing the trigger
t2=[]
for i,e in enumerate(ev):
    bars=sorted(json.load(open(f'{D}/entry_study/klines/{i}.json')))
    k0=next((j for j,b in enumerate(bars) if b[0]<=e['trig_ms']<b[0]+60000),None)
    if k0 is None: continue
    t2.append((e['setup'],bars,k0,e['direction']=='LONG',e['stop'],e['tp1']))
report('U0-C T2 (694 events, plan geometry)',t2)
# native: signal bar = first bar at/after snapshot ts
nat=[]
for r in json.load(open('u0b_rows.json')):
    i=byid[r['event_id']]; ts=datetime.datetime.fromisoformat(r['snap_ts']).timestamp()*1000
    bars=sorted(json.load(open(f'{D}/entry_study/klines/{i}.json'))); k0=next((j for j,b in enumerate(bars) if b[0]>=ts),None)
    if k0 is None: continue
    for s in r['final_signals'] or []:
        if s['stop']>0 and s['tp1']>0: nat.append((s['setup'],bars,k0,s['direction']=='LONG',s['stop'],s['tp1']))
report('U0-C native (final signals on the 694 snapshots, own geometry)',nat,minn=10)
