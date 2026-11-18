import json,random,collections
from common import pct
import os
D=os.environ['U0_DATA']
ev=json.load(open(D+'/entry_study/events.json'))
rep={x['index']:x for x in json.load(open('replay.json'))['records']}
rows={x['index']:x for x in json.load(open('u0f_rows.json'))}
order=sorted(range(len(ev)),key=lambda i:ev[i]['trig_ms'])
fold={i:1+rank*3//len(ev) for rank,i in enumerate(order)}
FEE=0.0005
FAST=lambda s:s.startswith('ignition_') or s.startswith('momentum_')
CLS={'momentum_continuation_long':'IMPULSE','momentum_continuation_short':'IMPULSE','momentum_continuation_watch_long':'IMPULSE','momentum_continuation_watch_short':'IMPULSE','ignition_early_long':'TRIGGER','ignition_early_short':'TRIGGER'}
lbc={}
def atr15(sym,t_ms):
    if sym not in lbc: lbc[sym]=json.load(open(f'{D}/lookback/{sym}_15m.json'))
    b=[r for r in lbc[sym] if r[0]+900000<=t_ms][-15:]
    if len(b)<15: return None
    trs=[max(b[k][2]-b[k][3],abs(b[k][2]-b[k-1][4]),abs(b[k][3]-b[k-1][4])) for k in range(1,15)]
    return sum(trs)/14
fills=[]
for i in sorted(rows):
    e=ev[i]; bars=json.load(open(f'{D}/entry_study/klines/{i}.json'))
    bars.sort(key=lambda b:b[0])
    fi=rep[i]['fill_idx']
    E,S,T1,T2=e['entry'],e['stop'],e['tp1'],e['tp2']; d=abs(E-S); sg=1 if e['direction']=='LONG' else -1
    a=atr15(e['symbol'],bars[fi][0])
    fills.append(dict(i=i,setup=e['setup'],fold=fold[i],E=E,d=d,sg=sg,T1=(T1-E)*sg/d,T2=(T2-E)*sg/d,atrR=(a/d if a else None),
        bars=[((b[2]-E)*sg/d if sg>0 else (b[3]-E)*sg/d, (b[3]-E)*sg/d if sg>0 else (b[2]-E)*sg/d, (b[4]-E)*sg/d, b[0]) for b in bars[fi:]],
        A=rows[i]['a_r'],Dr=rows[i]['d_r'],fast=FAST(e['setup'])))
def fee(f,legs): return FEE*(f['E']+sum(q*(f['E']+f['sg']*p*f['d']) for q,p in legs))/f['d']
def walk(f,v,prm):
    """generic walker; prices in R. bars: (fav, adv, close, open_ms)"""
    q=1.0; legs=[]; stop=-1.0; tp1=False; peak=0.0; t0=f['bars'][0][3]
    beR=fee(f,[(1,0)])  # R needed to cover round-trip fees
    for fav,adv,cl,t in f['bars']:
        age=(t-t0)/60000
        if adv<=stop: legs.append((q,stop)); q=0; break
        peak=max(peak,fav)
        if v=='E6':
            if fav>=f['T2']: legs.append((q,f['T2'])); q=0; break
            if age>=(60 if f['fast'] else 240): legs.append((q,cl)); q=0; break
            continue
        if not tp1 and fav>=f['T1']:
            tp1=True; legs.append((0.5,f['T1'])); q-=0.5
            if v in ('E1','E2'): stop=max(stop,beR)
        if tp1 and fav>=f['T2']: legs.append((q,f['T2'])); q=0; break
        if v=='E2' and not tp1 and peak>=beR+prm['buf']: stop=max(stop,beR)
        if v=='E3' and tp1 and f['atrR']: stop=max(stop,beR,peak-prm['k']*f['atrR'])
        if v=='E4' and not tp1 and age>=prm['T'] and cl<=0: legs.append((q,cl)); q=0; break
    if q>0: legs.append((q,f['bars'][-1][2]))
    return sum(qq*p for qq,p in legs)-fee(f,legs)
def params(train):
    w=[];post=[];tt=[]
    for f in train:
        reached=False;pk=0;dd=0;tp=False;pk2=0;dd2=0;t0=f['bars'][0][3]
        for fav,adv,cl,t in f['bars']:
            if adv<=-1: break
            if not tp:
                if reached: dd=max(dd,pk-adv)
                if fav>=0.25: reached=True
                if reached: pk=max(pk,fav)
                if fav>=f['T1']: tp=True; tt.append((t-t0)/60000); w.append(dd); continue
            else:
                pk2=max(pk2,fav); 
                if f['atrR']: dd2=max(dd2,(pk2-adv)/f['atrR'])
                if fav>=f['T2']: break
        if tp and f['atrR']: post.append(dd2)
    return dict(buf=pct(w,90) if w else 1, k=pct(post,90) if post else 1, T=pct(tt,90) if tt else 240)
def boot_p(xs,n=5000,seed=7):
    r=random.Random(seed);m=len(xs);c=0;ms=[]
    for _ in range(n):
        mu=sum(xs[r.randrange(m)] for _ in range(m))/m; ms.append(mu); c+=mu>0
    ms.sort(); return c/n,ms[int(n*.05)],ms[int(n*.95)-1]
def evaluate(v,by_class=False):
    out={}
    for k in (1,2,3):
        test=[f for f in fills if f['fold']==k]; train=[f for f in fills if f['fold']!=k]
        if v=='E5':
            for f in test:
                c=CLS.get(f['setup'],'OTHER'); tr=[g for g in train if CLS.get(g['setup'],'OTHER')==c] or train
                best=None
                for cand in ('E1','E2','E3','E4'):
                    # inner choice on the training folds of this class
                    pr=params(tr); s=sum(walk(g,cand,pr)-g['Dr'] for g in tr)/len(tr)
                    if best is None or s>best[0]: best=(s,cand,pr)
                out[f['i']]=walk(f,best[1],best[2])
        else:
            pr=params(train)
            for f in test: out[f['i']]=walk(f,v,pr) if v not in('A','D') else (f['A'] if v=='A' else f['Dr'])
    return out
res={}
print(f"{'variant':6s} {'mean R':>8s} {'dR vs D':>8s} {'90% CI':>18s} {'P(dR>0)':>8s}  folds dR (F1/F2/F3)     gate   {'dR vs A':>8s}")
for v in ['D','A','E1','E2','E3','E4','E5','E6']:
    o=evaluate(v); res[v]=o
    dv=[o[f['i']]-f['Dr'] for f in fills]; da=[o[f['i']]-f['A'] for f in fills]
    p,lo,hi=boot_p(dv) if v!='D' else (0,0,0)
    fd=[sum(o[f['i']]-f['Dr'] for f in fills if f['fold']==k)/sum(1 for f in fills if f['fold']==k) for k in (1,2,3)]
    mu=sum(dv)/len(dv); gate= (mu>=0.05 and p>=0.9 and all(x>0 for x in fd))
    print(f"{v:6s} {sum(o.values())/len(o):+8.4f} {mu:+8.4f} [{lo:+.4f},{hi:+.4f}] {p:8.3f}  {fd[0]:+.3f}/{fd[1]:+.3f}/{fd[2]:+.3f}   {'PASS' if gate else 'fail'}   {sum(da)/len(da):+8.4f}")
print('params on all folds (reference only):',params(fills), 'fills with ATR', sum(1 for f in fills if f['atrR']))
