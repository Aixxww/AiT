"""U0-F tables: K/S/N groups, A-D, folds, mechanisms, classes, distributions.
Input: u0f_rows.json from `U0F_RUN=1 go test ./trader -run TestU0F`."""
import json,collections
from common import data, pct, boot_ci, trigger_folds
r=json.load(open('u0f_rows.json')); ev=json.load(open(data('entry_study','events.json'))); fold=trigger_folds(ev)
prot=lambda x:x['a_exit']=='moved_stop' or x['a_exit'].startswith('protector_close')
grp=lambda x:'K' if prot(x) and x['p_first']=='tp1' else ('S' if prot(x) and x['p_first']=='stop' else 'N')
def line(name,rows):
    d=[x['a_r']-x['p_r'] for x in rows]; lo,hi=boot_ci(d)
    print(f"{name:34s} n={len(rows):3d} A {sum(x['a_r'] for x in rows)/len(rows):+.3f} P {sum(x['p_r'] for x in rows)/len(rows):+.3f} A-P {sum(d)/len(d):+.3f} [{lo:+.3f},{hi:+.3f}] sum {sum(d):+.2f}")
for g in 'KSN': line('group '+g,[x for x in r if grp(x)==g])
line('ALL',r)
ad=[x['a_r']-x['d_r'] for x in r]; lo,hi=boot_ci(ad); print(f"A-D {sum(ad)/len(ad):+.4f} [{lo:+.4f},{hi:+.4f}]  totals A {sum(x['a_r'] for x in r):+.2f} P {sum(x['p_r'] for x in r):+.2f} D {sum(x['d_r'] for x in r):+.2f}")
for f in (1,2,3):
    s=[x for x in r if fold[x['index']]==f]; print('fold',f,len(s),'A-D %+.4f'%(sum(x['a_r']-x['d_r'] for x in s)/len(s)))
w=[x for x in r if x['p_first']=='tp1']
pb=[x['pullback_r_before_tp1'] for x in w]; mt=[x['mins_to_tp1'] for x in w]
print('TP1-reaching plan paths n',len(w),'pullback p50/p90 %.3f/%.3f'%(pct(pb,50),pct(pb,90)),'minutes to TP1 p50/p90 %.0f/%.0f'%(pct(mt,50),pct(mt,90)))
