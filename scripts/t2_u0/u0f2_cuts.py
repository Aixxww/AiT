import json,collections
from common import boot_ci
import os
D=os.environ['U0_DATA']
ev=json.load(open(D+'/entry_study/events.json'))
F=json.load(open('u0f_rows.json'))
m1={r['index']:r for r in json.load(open('rrstage.json'))}  # t2_m2_eval -rr-min 1.5 -rr-stage-out
order=sorted(range(len(ev)),key=lambda i:ev[i]['trig_ms']); FOLD={i:1+k*3//len(ev) for k,i in enumerate(order)}
SHADOW={'ignition_early_short','momentum_continuation_watch_long','confirmed_breakout_long','confirmed_breakout_short'}
CLS=lambda s:'IMPULSE' if s.startswith('momentum_') else ('REVERSAL' if 'pullback' in s else 'TRIGGER')
BANDS=[(0,0.5),(0.5,0.8),(0.8,1.0),(1.0,1.2),(1.2,1.5),(1.5,99)]
def band(x):
    for a,b in BANDS:
        if a<=x<b: return (a,b)
# cell status from the MEAS1/U0 calibration (class x rr_meas band, fee-net R under OD-2)
cells=collections.defaultdict(list)
for r in m1.values():
    if r['stop_id']: cells[(CLS(r['setup']),band(r['rr_meas_tp1']))].append(r['fee_net_r'])
status={}
for k,v in cells.items():
    lo,hi=boot_ci(v)
    status[k]='EARNED' if len(v)>=30 and lo>0 else ('BLOCKED' if len(v)>=30 and hi<0 else 'PROBATION')
def cut_c1(x):
    r=m1.get(x['index'])
    if r is None: return False            # structurally inadmissible (no TP1 / no stop in band)
    c=CLS(x['setup']); st=status[(c,band(r['rr_meas_tp1']))]
    if st=='BLOCKED': return False
    if c=='TRIGGER' and r['rr_meas_tp1']<1.2: return False
    return True
def cut_c2(x): return x['setup'] not in SHADOW
BAD_SCORE=[(60,65),(90,95)]
def cut_c3(x): s=ev[x['index']]['score']; return not any(a<=s<b for a,b in BAD_SCORE)
CUTS={'C0 all 380 (U0-F)':lambda x:True,
      'C1 cell not BLOCKED + TRIGGER rr_meas>=1.2':cut_c1,
      'C2 registry-tradable families (U6a/OD-27/28)':cut_c2,
      'C3 drop U0-A score bands with CI<0':cut_c3,
      'C4 C2 and C3':lambda x:cut_c2(x) and cut_c3(x)}
def grp(x):
    prot=x['a_exit']=='moved_stop' or x['a_exit'].startswith('protector_close')
    return 'K' if prot and x['p_first']=='tp1' else ('S' if prot and x['p_first']=='stop' else 'N')
if __name__=='__main__':
    print('cell status:',{f'{k[0]} {k[1]}':(v,len(cells[k])) for k,v in sorted(status.items())})
    for name,f in CUTS.items():
        s=[x for x in F if f(x)]
        d=[x['a_r']-x['p_r'] for x in s]; lo,hi=boot_ci(d) if d else (0,0)
        flag='' if len(s)>=60 else '  ** UNDERPOWERED (n<60) **'
        print(f'\n{name}: n={len(s)}{flag}')
        if not s: continue
        print(f'  A {sum(x["a_r"] for x in s)/len(s):+.3f}  P {sum(x["p_r"] for x in s)/len(s):+.3f}  A-P {sum(d)/len(d):+.3f} [{lo:+.3f},{hi:+.3f}]  totals A {sum(x["a_r"] for x in s):+.1f} P {sum(x["p_r"] for x in s):+.1f} A-P {sum(d):+.1f}')
        for g in 'KSN':
            t=[x for x in s if grp(x)==g]
            if t: print(f'  {g}: n={len(t):3d} ({100*len(t)/len(s):4.1f}%)  A {sum(x["a_r"] for x in t)/len(t):+.3f}  P {sum(x["p_r"] for x in t)/len(t):+.3f}  sum(A-P) {sum(x["a_r"]-x["p_r"] for x in t):+.1f}')
        print('  folds n:',[sum(FOLD[x['index']]==k for x in s) for k in (1,2,3)], ' A-P by fold:',['%+.3f'%(sum(x['a_r']-x['p_r'] for x in s if FOLD[x['index']]==k)/max(1,sum(FOLD[x['index']]==k for x in s))) for k in (1,2,3)])
