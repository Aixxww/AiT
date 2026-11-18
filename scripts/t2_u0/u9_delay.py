"""U9 acceptance: delay cost per class of the conditional entry (owner option A:
entry at the close of the first completed 1m candle that closes through the
trigger) against the current market path, with the U0-C method.

Reference (U0-C base): market entry at the close of the 1m bar containing the
upstream trigger (trig_ms). Delay cost = R(path) - R(base), paired, same plan
stop / TP1, OD-2 fee-net R; run-away = TP1 already passed at entry.
Paths:
  current Δd      : market entry d = 1..5 min after the base bar (U0-C grid).
  current path    : the per-event mean over d = 2, 3, 4: the current path
                    decides at TRIGGERED, i.e. at the next 3-min scan (phase
                    unknown: 1-3 min) plus about one bar of LLM round trip.
  conditional     : pending from the base bar on; a bar touching the stop first
                    cancels it; the first bar closing through the trigger
                    enters at its close; expiry 120 min (config default).
Input: u5 rows (cmd/t2_u5_tags) for the signal class of each event.
Usage: u9_delay.py u5_48h.json"""
import json,sys,collections as C
from common import data, boot_ci
COST=0.0015; EXPIRY=120
ev=json.load(open(data('entry_study','events.json')))
cls={r['event_id']:r['states'][-1]['record']['signal_class'] for r in json.load(open(sys.argv[1])) if r.get('states')}
def outcome(bars,k,L,stop,tp1):
    if k is None or k>=len(bars): return ('nodata',None)
    E=bars[k][4]
    if (L and E>=tp1) or ((not L) and E<=tp1): return ('runaway',None)
    if (L and E<=stop) or ((not L) and E>=stop): return ('invalidated',None)
    rew=(abs(tp1-E)-E*COST)/(abs(E-stop)+E*COST)
    for b in bars[k+1:]:
        if (b[3]<=stop) if L else (b[2]>=stop): return ('stop',-1.0)
        if (b[2]>=tp1) if L else (b[3]<=tp1): return ('tp1',rew)
    return ('none',0.0)
def conditional(bars,k0,L,trig,stop):
    for j in range(k0,min(len(bars),k0+EXPIRY)):
        b=bars[j]
        if (b[3]<=stop) if L else (b[2]>=stop): return 'cancelled',None
        if (b[4]>=trig) if L else (b[4]<=trig): return 'filled',j
    return 'expired',None
rows=[]
for i,e in enumerate(ev):
    bars=sorted(json.load(open(data('entry_study','klines',f'{i}.json'))))
    k0=next((j for j,b in enumerate(bars) if b[0]<=e['trig_ms']<b[0]+60000),None)
    if k0 is None: continue
    L=e['direction']=='LONG'; base=outcome(bars,k0,L,e['stop'],e['tp1'])
    st,kc=conditional(bars,k0,L,e['entry'],e['stop'])
    r=dict(cls=cls.get(e['event_id'],'?'),setup=e['setup'],base=base,cond_state=st,
           cond=outcome(bars,kc,L,e['stop'],e['tp1']) if kc is not None else (st,None),
           cond_delay=(kc-k0) if kc is not None else None,
           **{f'd{d}':outcome(bars,k0+d,L,e['stop'],e['tp1']) for d in range(1,6)})
    rows.append(r)
def cost(rs,key):
    x=[r[key][1]-r['base'][1] for r in rs if r['base'][1] is not None and r[key][1] is not None]
    ra=sum(1 for r in rs if r[key][0]=='runaway')
    if not x: return 'n=0'
    lo,hi=boot_ci(x)
    return f'{sum(x)/len(x):+.3f} [{lo:+.3f},{hi:+.3f}] n={len(x)} runaway {ra}'
def cost_cur(rs):
    x=[]
    for r in rs:
        if r['base'][1] is None: continue
        v=[r[f'd{d}'][1] for d in (2,3,4)]
        if any(y is None for y in v): continue
        x.append(sum(v)/3-r['base'][1])
    if not x: return 'n=0'
    lo,hi=boot_ci(x)
    return f'{sum(x)/len(x):+.3f} [{lo:+.3f},{hi:+.3f}] n={len(x)}'
def population(rs):
    """Mean R per event over all events of the group, an event not entered
    (run-away, invalidated, cancelled, expired, no data) counting 0."""
    def m(f):
        x=[f(r) for r in rs]; lo,hi=boot_ci(x); return f'{sum(x)/len(x):+.3f} [{lo:+.3f},{hi:+.3f}]'
    cur=lambda r: sum((r[f'd{d}'][1] or 0.0) for d in (2,3,4))/3
    return f'base {m(lambda r:r["base"][1] or 0.0)} | current path {m(cur)} | conditional {m(lambda r:r["cond"][1] or 0.0)}'
def report(name,rs):
    st=C.Counter(r['cond_state'] for r in rs)
    dl=sorted(r['cond_delay'] for r in rs if r['cond_delay'] is not None)
    med=dl[len(dl)//2] if dl else None
    print(f'\n== {name}: n={len(rs)}  conditional filled {st["filled"]} / cancelled {st["cancelled"]} / expired {st["expired"]}; fill delay vs base bar (min) median {med}, p90 {dl[int(len(dl)*0.9)] if dl else None}')
    for d in range(1,6):
        print(f'   delay cost  current Δ{d}: {cost(rs,f"d{d}")}')
    print(f'   delay cost  current path (mean Δ2-Δ4): {cost_cur(rs)}')
    print(f'   delay cost  conditional: {cost(rs,"cond")}')
    print(f'   population mean R per event (not entered = 0): {population(rs)}')
    for s_ in ('cancelled','expired'):
        b=[r['base'][1] for r in rs if r['cond_state']==s_ and r['base'][1] is not None]
        if b: print(f'   {s_}: base-path R of these events (not entered by the conditional path) mean {sum(b)/len(b):+.3f} n={len(b)}')
print('events',len(rows))
for c in ['TRIGGER','IMPULSE','REVERSAL']:
    report('class '+c+(' (conditional class)' if c=='TRIGGER' else ' (not conditional by default; shown for reference)'),[r for r in rows if r['cls']==c])
for s,n in C.Counter(r['setup'] for r in rows if r['cls']=='TRIGGER').most_common():
    if n>=15: report('  TRIGGER family '+s,[r for r in rows if r['setup']==s])
