import json,collections,datetime,bisect,os
C=collections.Counter
L=os.environ['U0_DATA']+'/live_db/'
P=[json.loads(l) for l in open(L+'trader_positions.jsonl')]
O=[json.loads(l) for l in open(L+'trader_orders.jsonl')]
FI=[json.loads(l) for l in open(L+'trader_fills.jsonl')]
iso=lambda s:datetime.datetime.fromisoformat(s.replace('Z','+00:00')).timestamp()*1000
opens=[];closes=[]
for l in open('decision_records.jsonl'):
    x=json.loads(l)
    try: ds=json.loads(x['decisions'] or 'null') or []
    except Exception: ds=[]
    try: dj=json.loads(x['decision_json'] or 'null') or []
    except Exception: dj=[]
    djm={(y.get('symbol'),y.get('action')):y for y in dj if isinstance(y,dict)}
    hv='hunter_v7' in (x.get('system_prompt') or '')[:200000]
    for y in ds:
        a=y.get('action','')
        if not y.get('success'): continue
        t=iso(y['timestamp']) if y.get('timestamp') else iso(x['timestamp'])
        rec=dict(trader=x['trader_id'],symbol=y['symbol'],t=t,hv=hv,price=y.get('price'),stop=y.get('stop_loss'),tp=y.get('take_profit'),lev=y.get('leverage'),
                 sel=(djm.get((y['symbol'],a)) or {}).get('selected_hunter_v7_signal_id'),setup=(djm.get((y['symbol'],a)) or {}).get('selected_hunter_v7_setup'))
        if a.startswith('open'): rec['side']='LONG' if a=='open_long' else 'SHORT'; opens.append(rec)
        elif a.startswith('close'): rec['side']='LONG' if a=='close_long' else 'SHORT'; closes.append(rec)
print('successful decision opens',len(opens),'(hunter_v7 prompt:',sum(o['hv'] for o in opens),') closes',len(closes))
# match each open decision to a position (same trader, symbol, side, entry within 3 min)
used=set();rows=[]
for o in opens:
    best=None
    for p in P:
        if p['id'] in used or p['trader_id']!=o['trader'] or p['symbol']!=o['symbol'] or p['side']!=o['side']: continue
        dt=abs(p['entry_time']-o['t'])
        if dt<=180000 and (best is None or dt<best[0]): best=(dt,p)
    if best: used.add(best[1]['id']); rows.append((o,best[1]))
print('open decisions matched to a position',len(rows),'| positions without a decision open',len(P)-len(used),'(leverage among them:',C(p['leverage'] for p in P if p['id'] not in used).most_common(4),')')
# fills for fee and partial-close counting
ords_by=collections.defaultdict(list)
for x in O: ords_by[(x['trader_id'],x['symbol'],x['position_side'])].append(x)
fee_by_order={x['exchange_order_id']:x['commission'] for x in O}
SIG=collections.defaultdict(list)
for l in open('hunter_v7_signal_records_trimmed.jsonl'):
    r=json.loads(l); SIG[(r['symbol'],r['direction'])].append((iso(r['timestamp']),r['setup_type'],r['ai_priority'],r.get('execution_tier')))
for k in SIG: SIG[k].sort()
out=[]
for o,p in rows:
    if p['status']!='CLOSED' or not o['stop'] or not p['exit_price']: continue
    E=p['entry_price']; S=o['stop']; T=o['tp']; L_=p['side']=='LONG'; sg=1 if L_ else -1
    d=abs(E-S)
    if d<=0: continue
    q=p['entry_quantity']
    cl=[x for x in ords_by[(p['trader_id'],p['symbol'],p['side'])] if x['order_action'].startswith('close') and p['entry_time']<=x['created_at']<=(p['exit_time'] or 0)+60000]
    fees=fee_by_order.get(p['entry_order_id'],0)+sum(x['commission'] for x in cl)
    if not cl: fees+= p['fee'] or 0
    gross=sum(sg*(x['avg_fill_price']-E)*x['filled_quantity'] for x in cl) if cl else sg*(p['exit_price']-E)*q
    R=(gross-fees)/(q*d)
    X=p['exit_price']; ux=sg*(X-E)/d
    # exit mechanism attribution
    lc=[c for c in closes if c['trader']==p['trader_id'] and c['symbol']==p['symbol'] and c['side']==p['side'] and abs(c['t']-p['exit_time'])<=120000]
    if p['close_reason'] and p['close_reason']!='sync': mech='protector:'+p['close_reason']
    elif lc: mech='llm_close'
    elif abs(ux+1)<=0.10: mech='stop_at_plan'
    elif T and abs(X-T)/E<=0.0015: mech='tp_at_plan'
    elif -0.1<=ux<=0.3: mech='moved_stop_or_breakeven (inferred)'
    elif len(cl)>1: mech='protector_partials (inferred)'
    else: mech='unspecified'
    s=SIG.get((p['symbol'],p['side']),[]); k=bisect.bisect_right(s,(p['entry_time'],'~'))
    sig=s[k-1] if k and p['entry_time']-s[k-1][0]<=15*60000 else None
    out.append(dict(pos=p['id'],trader=p['trader_id'],symbol=p['symbol'],side=p['side'],lev=p['leverage'],entry=E,stop=S,tp=T,stop_pct=d/E*100,entry_ms=p['entry_time'],exit_ms=p['exit_time'],exit=X,ux=ux,R=R,mech=mech,n_close_orders=len(cl),
                    setup=o['setup'] or (sig[1] if sig else None),setup_src='decision' if o['setup'] else ('signal_table' if sig else None),hv=o['hv']))
json.dump(out,open('u0h_rows.json','w'))
print('\nclosed matched trades with plan stop:',len(out))
print('setup source',C(x['setup_src'] for x in out))
print('exit mechanism',C(x['mech'] for x in out).most_common())
print('exits within +-0.1R fee-net',sum(abs(x['R'])<=0.1 for x in out),'by mechanism',C(x['mech'] for x in out if abs(x['R'])<=0.1).most_common())
print('leverage',C(x['lev'] for x in out).most_common(),'stop% median',sorted(x['stop_pct'] for x in out)[len(out)//2])
print('multi-close-order (partial) positions',sum(x['n_close_orders']>1 for x in out))
print('mean fee-net R',round(sum(x['R'] for x in out)/len(out),3),'sum',round(sum(x['R'] for x in out),2))
for m,n in C(x['mech'] for x in out).most_common(): 
    s=[x['R'] for x in out if x['mech']==m]; print(f'  {m:38s} n={n:3d} mean R {sum(s)/n:+.3f}')
print('setups',C(x['setup'] for x in out).most_common(12))
