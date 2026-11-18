import json,random
from u0f2_cuts import CUTS,FOLD,F
import os
D=os.environ['U0_DATA']
ev=json.load(open(D+'/entry_study/events.json'))
rep={x['index']:x for x in json.load(open('replay.json'))['records']}
TPCAP=7.75  # % price: kernel.HunterV7EffectiveExecutionGeometry(3,0,0.5,3,true).MaxTPMovePct, as in U0-F
FEE=0.0005
def prep(i):
    e=ev[i]; bars=sorted(json.load(open(f'{D}/entry_study/klines/{i}.json'))); fi=rep[i]['fill_idx']
    E,S=e['entry'],e['stop']; d=abs(E-S); L=e['direction']=='LONG'; sg=1 if L else -1
    u=lambda p:sg*(p-E)/d
    tp=e['tp1']; cap=E*(1+sg*TPCAP/100)
    if sg*(tp-cap)>0: tp=cap
    B=[(u(b[2] if L else b[3]),u(b[3] if L else b[2]),u(b[4]),b[0]) for b in bars[fi:]]
    return dict(E=E,d=d,sg=sg,tp=u(tp),B=B,fast=e['setup'].startswith(('ignition_','momentum_')),pct=lambda r:r*d/E*100)
def fee(f,legs): return FEE*(f['E']+sum(q*(f['E']+f['sg']*p*f['d']) for q,p in legs))/f['d']
def done(f,p): return p-fee(f,[(1,p)])
def V1(f):  # Nofx: static SL+TP; full close at poll when price move >5% and giveback>=40% of peak profit
    peak=None
    for fav,adv,cl,t in f['B']:
        if adv<=-1: return done(f,-1)
        if fav>=f['tp']: return done(f,f['tp'])
        pnl=cl  # leveraged pnl is proportional to R; giveback ratio is leverage-free
        peak=pnl if peak is None else max(peak,pnl)
        dd=(peak-pnl)/peak*100 if peak>0 and pnl<peak else 0
        if f['pct'](cl)>5.0 and dd>=40: return done(f,cl)
    return done(f,f['B'][-1][2])
def V2(f):  # freqtrade trailing_only_offset_is_reached: offset 1.0R, trail 0.5R off highest observed price; ratchet; full exit
    stop=-1.0; peak=0.0
    for fav,adv,cl,t in f['B']:
        if adv<=stop: return done(f,stop)
        if fav>=f['tp']: return done(f,f['tp'])
        peak=max(peak,fav)
        if peak>=1.0: stop=max(stop,peak-0.5)
    return done(f,f['B'][-1][2])
def V3(f):  # Hummingbot triple barrier: SL, TP1, time limit 60/240m; trailing on fee-net pnl, activation 1.0R, delta 0.5R, at poll
    trig=None; t0=f['B'][0][3]
    for fav,adv,cl,t in f['B']:
        if adv<=-1: return done(f,-1)
        if fav>=f['tp']: return done(f,f['tp'])
        net=done(f,cl)
        if trig is None:
            if net>1.0: trig=net-0.5
        else:
            if net<trig: return net
            if net-0.5>trig: trig=net-0.5
        if (t-t0)/60000>=(60 if f['fast'] else 240): return net
    return done(f,f['B'][-1][2])
P={x['index']:prep(x['index']) for x in F}
def boot_p(xs,n=5000,seed=11):
    r=random.Random(seed); m=len(xs); c=0; ms=[]
    for _ in range(n):
        mu=sum(xs[r.randrange(m)] for _ in range(m))/m; ms.append(mu); c+=mu>0
    ms.sort(); return c/n,ms[int(n*.05)],ms[int(n*.95)-1]
if __name__=="__main__":
    for cname,cf in CUTS.items():
        s=[x for x in F if cf(x)]
        print(f'\n=== {cname}  n={len(s)}' + ('  ** UNDERPOWERED **' if len(s)<60 else ''))
        ap=sum(x['a_r']-x['p_r'] for x in s)/len(s); print(f'  context: A-P {ap:+.3f}')
        for vn,vf in (('V1 Nofx giveback',V1),('V2 freqtrade offset-trail',V2),('V3 Hummingbot triple-barrier',V3)):
            v={x['index']:vf(P[x['index']]) for x in s}
            dA=[v[x['index']]-x['a_r'] for x in s]; dP=[v[x['index']]-x['p_r'] for x in s]
            p,lo,hi=boot_p(dA); mu=sum(dA)/len(dA)
            fd=[sum(v[x['index']]-x['a_r'] for x in s if FOLD[x['index']]==k)/max(1,sum(FOLD[x['index']]==k for x in s)) for k in (1,2,3)]
            g=mu>=0.05 and p>=0.9 and all(z>0 for z in fd)
            print(f'  {vn:30s} mean {sum(v.values())/len(v):+.3f}  dR vs A {mu:+.3f} [{lo:+.3f},{hi:+.3f}] P={p:.2f} folds {fd[0]:+.3f}/{fd[1]:+.3f}/{fd[2]:+.3f}  {"PASS" if g else "fail"}  | vs P {sum(dP)/len(dP):+.3f}')
