# FROZEN RECORD of the single U0-H2 run (2026-11-18). Do not re-run for a decision;
# see docs/t2-u0-preregistrations.md. Reads u0h2_trades.json / u0h2_A.json from the working directory.
# U0-H2 single run: V2 (frozen U0-F2 spec), stats and gate. Reads A from the Go runner.
import json,random
FEE=0.0005; TPCAP=7.75
T=[t for t in json.load(open('u0h2_trades.json')) if t['pos'] not in (9,11)]
A={r['pos']:r for r in json.load(open('u0h2_A.json'))}
assert len(T)==136 and set(A)=={t['pos'] for t in T}
def prep(t):
    E,S=t['entry'],t['stop']; d=abs(E-S); L=t['side']=='LONG'; sg=1 if L else -1
    u=lambda p:sg*(p-E)/d
    tp=t['tp']; cap=E*(1+sg*TPCAP/100)
    if sg*(tp-cap)>0: tp=cap
    B=[(u(b[2] if L else b[3]),u(b[3] if L else b[2]),u(b[4]),b[0]) for b in t['bars']]
    return dict(E=E,d=d,sg=sg,tp=u(tp),B=B)
def fee(f,legs): return FEE*(f['E']+sum(q*(f['E']+f['sg']*p*f['d']) for q,p in legs))/f['d']
def done(f,p): return p-fee(f,[(1,p)])
def V2(f):  # frozen U0-F2 spec
    stop=-1.0; peak=0.0
    for fav,adv,cl,t in f['B']:
        if adv<=stop: return done(f,stop)
        if fav>=f['tp']: return done(f,f['tp'])
        peak=max(peak,fav)
        if peak>=1.0: stop=max(stop,peak-0.5)
    return done(f,f['B'][-1][2])
def Pw(f):  # plan, context only: stop or TP, full exit, never moved
    for fav,adv,cl,t in f['B']:
        if adv<=-1: return done(f,-1)
        if fav>=f['tp']: return done(f,f['tp'])
    return done(f,f['B'][-1][2])
rows=[]
for t in T:
    f=prep(t); rows.append(dict(pos=t['pos'],ms=t['entry_ms'],mech=t['mech'],real=t['R'],A=A[t['pos']]['a_r'],V2=V2(f),P=Pw(f)))
rows.sort(key=lambda r:r['ms']); n=len(rows)
for k,r in enumerate(rows): r['fold']=1+k*3//n
d=[r['V2']-r['A'] for r in rows]; mu=sum(d)/n
rnd=random.Random(20261118); c=0; ms=[]
for _ in range(5000):
    m=sum(d[rnd.randrange(n)] for _ in range(n))/n; ms.append(m); c+=m>0
ms.sort(); P=c/5000
fd=[sum(r['V2']-r['A'] for r in rows if r['fold']==k)/sum(r['fold']==k for r in rows) for k in (1,2,3)]
g1,g2,g3=mu>=0.05,P>=0.9,all(x>0 for x in fd)
print('=== U0-H2 RESULT (single run) ===  n =',n,' fold sizes',[sum(r['fold']==k for r in rows) for k in (1,2,3)])
print(f'mean V2-A {mu:+.4f}  90% CI [{ms[250]:+.4f},{ms[4749]:+.4f}]  P(mean>0) {P:.4f}  folds {fd[0]:+.4f} / {fd[1]:+.4f} / {fd[2]:+.4f}')
print(f'gate: mean>=+0.05 {g1} | P>=0.9 {g2} | all folds>0 {g3}  =>  {"PASS" if g1 and g2 and g3 else "FAIL"}')
for k in ('A','V2','P','real'): print(f'  total {k:4s} {sum(r[k] for r in rows):+8.2f} R   mean {sum(r[k] for r in rows)/n:+.4f}')
fid=sum(r['A']-r['real'] for r in rows)/n
xa=[r['A'] for r in rows]; xr=[r['real'] for r in rows]; ma=sum(xa)/n; mr=sum(xr)/n
cor=sum((a-ma)*(b-mr) for a,b in zip(xa,xr))/((sum((a-ma)**2 for a in xa)*sum((b-mr)**2 for b in xr))**.5)
print(f'sim fidelity: mean(simA - realized) {fid:+.4f}  corr {cor:+.3f}  ->',('LOW FIDELITY' if abs(fid)>0.15 else 'within 0.15R'))
print(f'context: mean V2-P {sum(r["V2"]-r["P"] for r in rows)/n:+.4f}  mean A-P {sum(r["A"]-r["P"] for r in rows)/n:+.4f}')
import collections
for m,k in collections.Counter(r['mech'] for r in rows).most_common():
    s=[r for r in rows if r['mech']==m]; print(f'  by live mechanism {m:38s} n={k:3d} V2-A {sum(r["V2"]-r["A"] for r in s)/k:+.3f}')
json.dump(rows,open('u0h2_result_rows.json','w'))
