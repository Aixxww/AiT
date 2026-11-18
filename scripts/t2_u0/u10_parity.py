"""U10 partial parity test (owner decision: option 2).

For live Hunter v7 signal records, rebuild Binance USD-M 1m klines from the
data.binance.vision daily archives and recompute the record's stored price
fields under the two feature modes:
  forming : change_1h = (P(tau) - C1h_prev) / C1h_prev, P = price at tau,
            C1h_prev = close of the last closed 1h bar (the live code: last
            1h bar is the forming one); change_4h the same on 4h bars
  closed  : change_1h = (C1h_last_closed - C1h_before) / C1h_before; 4h same
tau is the time the live snapshot was taken, unknown but no later than the
record timestamp T. Both modes get the same search window,
tau in {T - k min, k = 0..WINDOW_MIN}, and the best match per record is
reported, plus the value at tau = T. P(tau) is the close of the last 1m bar
that closed at or before tau (minute resolution).
change_24h comes from the 24h ticker in live code (mode-independent); it is
checked against the 1m archive as context only.
LIMITATION (recorded, not worked around): OI, funding rate and every score
have no archive source and are not checked.
Sample: unique (timestamp, symbol) snapshots with a non-zero change_1h or
change_4h, N = SAMPLE drawn with random.Random(SEED). Archives are cached
in ./kl. Input: hunter_v7_signal_records_trimmed.jsonl in the working dir.
Usage: u10_parity.py [N]"""
import json,sys,os,io,zipfile,random,urllib.request,datetime as dt,collections as C
SEED=20261118; SAMPLE=int(sys.argv[1]) if len(sys.argv)>1 else 300; WINDOW_MIN=5
TOL=(0.01,0.05,0.2)  # percentage-point tolerances reported
keys={}
for line in open('hunter_v7_signal_records_trimmed.jsonl'):
    r=json.loads(line)
    c1,c4=r.get('change_1h') or 0,r.get('change_4h') or 0
    if c1==0 and c4==0: continue
    k=(r['timestamp'],r['symbol'])
    if k not in keys: keys[k]=(c1,c4,r.get('change_24h'))
pool=sorted(keys); rnd=random.Random(SEED); sample=rnd.sample(pool,min(SAMPLE,len(pool)))
os.makedirs('kl',exist_ok=True); cache={}
def day_bars(sym,day):
    if (sym,day) in cache: return cache[(sym,day)]
    fn=f'kl/{sym}-1m-{day}.zip'; bars={}
    if not os.path.exists(fn):
        url=f'https://data.binance.vision/data/futures/um/daily/klines/{sym}/1m/{sym}-1m-{day}.zip'
        try: open(fn,'wb').write(urllib.request.urlopen(url,timeout=30).read())
        except Exception: open(fn+'.missing','w').close() if not os.path.exists(fn+'.missing') else None
    if os.path.exists(fn):
        with zipfile.ZipFile(fn) as z:
            for line in z.read(z.namelist()[0]).decode().splitlines():
                p=line.split(',')
                if not p[0].isdigit(): continue
                bars[int(p[0])]=(float(p[1]),float(p[4]))  # open time -> (open, close)
    cache[(sym,day)]=bars; return bars
def bars_for(sym,t_ms):
    out={}
    for d in (-1,0):
        day=(dt.datetime.fromtimestamp(t_ms/1000,dt.timezone.utc)+dt.timedelta(days=d)).strftime('%Y-%m-%d')
        out.update(day_bars(sym,day))
    return out
M=60000
def close_at(b,t):  # close of the last 1m bar closed at or before t (ms)
    m=(t//M)*M-M
    return b.get(m,(None,None))[1]
def bar_close(b,start,length):  # close of the [start, start+length) bar = close of its last minute
    return b.get(start+length-M,(None,None))[1]
def changes(b,tau):
    out={}
    for name,L in (('change_1h',3600000),('change_4h',4*3600000)):
        cur=(tau//L)*L                          # forming bar start
        last_closed=bar_close(b,cur-L,L); before=bar_close(b,cur-2*L,L); p=close_at(b,tau)
        out[name]={'forming':(p-last_closed)/last_closed*100 if p and last_closed else None,
                   'closed':(last_closed-before)/before*100 if last_closed and before else None}
    o24=b.get(((tau-86400000)//M)*M,(None,None))[0]; p=close_at(b,tau)
    out['change_24h']=(p-o24)/o24*100 if p and o24 else None
    return out
res=C.defaultdict(lambda:C.defaultdict(list)); used=0; skipped=0
for ts,sym in sample:
    T=int(dt.datetime.fromisoformat(ts.replace(' ','T')).timestamp()*1000)
    b=bars_for(sym,T)
    if not b: skipped+=1; continue
    rec=dict(zip(('change_1h','change_4h','change_24h'),keys[(ts,sym)]))
    taus=[T-k*M for k in range(WINDOW_MIN+1)]; vals=[changes(b,t) for t in taus]
    ok=False
    for f in ('change_1h','change_4h'):
        if not rec[f]: continue
        for mode in ('forming','closed'):
            d=[abs(v[f][mode]-rec[f]) for v in vals if v[f][mode] is not None]
            if not d: continue
            res[f][mode+'_best'].append(min(d)); res[f][mode+'_atT'].append(d[0]); ok=True
        e=[abs(v[f]['forming']-v[f]['closed']) for v in vals[:1] if v[f]['forming'] is not None and v[f]['closed'] is not None]
        res[f]['effect'].extend(e)
    if rec['change_24h'] is not None:
        d=[abs(v['change_24h']-rec['change_24h']) for v in vals if v['change_24h'] is not None]
        if d: res['change_24h']['ticker_best'].append(min(d))
    used+=ok; skipped+=not ok
def q(x,p): s=sorted(x); return s[min(len(s)-1,int(p*len(s)))]
print(f'sample {len(sample)} snapshots (of {len(pool)}), used {used}, skipped (no archive or no comparable field) {skipped}; window tau in T-0..{WINDOW_MIN} min')
for f in ('change_1h','change_4h'):
    print(f'\n{f}  |recorded - recomputed| in percentage points')
    for k in ('forming_best','closed_best','forming_atT','closed_atT'):
        x=res[f][k]
        if x: print(f'  {k:13s} n={len(x):4d} p50 {q(x,.5):.4f} p90 {q(x,.9):.4f} '+' '.join(f'<= {t}: {100*sum(v<=t for v in x)/len(x):5.1f}%' for t in TOL))
    x=res[f]['effect']
    if x: print(f'  forming-bar effect |forming - closed| at tau=T: n={len(x)} p50 {q(x,.5):.3f} p90 {q(x,.9):.3f} mean {sum(x)/len(x):.3f} pp')
x=res['change_24h']['ticker_best']
if x: print(f'\nchange_24h (context; ticker, mode-independent) best |diff| n={len(x)} p50 {q(x,.5):.4f} p90 {q(x,.9):.4f} <= 0.2: {100*sum(v<=0.2 for v in x)/len(x):.1f}%')
print('\nNot checked (no archive source): OI, funding rate, scores.')
