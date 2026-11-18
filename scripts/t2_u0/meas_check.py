"""Check 6 reproductions: MEAS1 (rr-stage rows vs measure_rr.json) and MEAS2
(u0b_rows.json T2 fields vs measure_tiering.json). Inputs: rrstage.json from
t2_m2_eval -rr-min 1.5 -rr-stage-out, u0b_rows.json from cmd/t2_u0_native."""
import json
from common import data, pct
ref=json.load(open(data('measure','measure_rr.json')))['per_event_rr_stage']
a={r['index']:r for r in json.load(open('rrstage.json'))}
bad=sum(any(a[q['index']][k]!=q[k] for k in ['rr_meas_tp1','stop_id','stop','tp1','first','stop_then_tp1','fee_net_r']) or a[q['index']]['admitted']!=q['admitted_at_1.5'] for q in ref)
rej=[r['rr_meas_tp1'] for r in a.values() if not r['admitted']]
print('MEAS1 rows',len(a),'mismatches',bad,'median rr_meas %.3f'%pct(rej,50),'admitted at 1.2/1.0/0.5',[sum(r['rr_meas_tp1']>=f for r in a.values()) for f in (1.2,1.0,0.5)])
new=json.load(open('u0b_rows.json')); t=json.load(open(data('measure','measure_tiering.json')))['events']
F=['t2_detected','t2_func_status','t2_effective_status','t2_effective_status_nil_regime','t2_observe_only','t2_score','candidate_status','candidate_tier','candidate_tier_reason','store_initial_status','store_expiry_hours']
# MEAS2 was measured on pre-Task-E code (c381f1b; reproduces exactly at
# 1d9fb38). From U2 on, T2 candidates use the 1.2 RR floor, which changes
# only candidate_tier_reason (backend_rr_infeasible -> needs_confirmation);
# every other MEAS2 field must still match.
import collections
same=[k for k in F if k!='candidate_tier_reason']
print('MEAS2 field mismatches (excluding candidate_tier_reason)',sum(x.get(k)!=y.get(k) for x,y in zip(new,t) for k in same),'over',len(new),'events')
print('tier reasons at this code:',dict(collections.Counter((x['candidate_tier'],x['candidate_tier_reason']) for x in new)))
