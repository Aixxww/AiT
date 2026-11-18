"""U0-D: early stop-out (within 3 x 15m bars) by stop / ATR14(15m) band, per class."""
import json,math
from common import data
ev=json.load(open(data('entry_study','events.json')))
CLS=lambda s:'IMPULSE' if s.startswith('momentum_') else ('REVERSAL' if 'pullback' in s else 'TRIGGER')
lb={}
def atr(sym,t):
    if sym not in lb: lb[sym]=json.load(open(data('lookback',f'{sym}_15m.json')))
    b=[r for r in lb[sym] if r[0]+900000<=t][-15:]
    if len(b)<15: return None
    return sum(max(b[j][2]-b[j][3],abs(b[j][2]-b[j-1][4]),abs(b[j][3]-b[j-1][4])) for j in range(1,15))/14
rows=[]
for i,e in enumerate(ev):
    bars=sorted(json.load(open(data('entry_study','klines',f'{i}.json'))))
    k0=next((j for j,b in enumerate(bars) if b[0]<=e['trig_ms']<b[0]+60000),None)
    if k0 is None: continue
    E=bars[k0][4]; L=e['direction']=='LONG'; S=e['stop']
    if (L and E<=S) or ((not L) and E>=S): continue
    a=atr(e['symbol'],bars[k0][0]+60000)
    if not a: continue
    early=False
    for b in bars[k0+1:]:
        if (b[3]<=S) if L else (b[2]>=S): early=(b[0]-bars[k0][0])<=45*60000; break
    rows.append(dict(cls=CLS(e['setup']),k=abs(E-S)/a,early=early))
bands=[(0,0.5),(0.5,1),(1,1.5),(1.5,2),(2,3),(3,99)]
def z(p1,n1,p2,n2):
    p=(p1*n1+p2*n2)/(n1+n2); se=math.sqrt(p*(1-p)*(1/n1+1/n2)) if 0<p<1 else 0
    return (p1-p2)/se if se else 0
for c in ['ALL','IMPULSE','TRIGGER','REVERSAL']:
    s=[r for r in rows if c=='ALL' or r['cls']==c]
    st=[(lo,hi,len(t),sum(r['early'] for r in t)/len(t)) for lo,hi in bands for t in [[r for r in s if lo<=r['k']<hi]] if t]
    floor=None
    print(f'\n{c} n={len(s)}')
    for j,(lo,hi,n,p) in enumerate(st):
        nx=st[j+1] if j+1<len(st) else None
        sig=nx is not None and n>=15 and nx[2]>=15 and z(p,n,nx[3],nx[2])>1.2816
        print(f'  [{lo},{hi}) n={n} early {100*p:.1f}%'+(f' z={z(p,n,nx[3],nx[2]):+.2f}' if nx else ''))
        if floor is None and not sig and n>=15: floor=lo
    print('  rule output floor:',floor)
