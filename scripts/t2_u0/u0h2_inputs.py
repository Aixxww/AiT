"""U0-H2 inputs: per-trade 1m paths (Binance USD-M daily archives from
data.binance.vision, entry minute to entry + 12 h) and decision leverage for
the live trades in u0h_rows.json (from u0h_attribution.py). Writes
u0h2_trades.json; archives are cached in ./kl. Stops on any input that does
not meet the pre-registration rather than repairing it."""
import json,zipfile,csv,io,datetime,os,subprocess,collections
src=open(os.path.join(os.path.dirname(__file__),'u0h_attribution.py')).read().split("print('successful decision opens'")[0]
g={}; exec(src,g)
r=json.load(open('u0h_rows.json')); os.makedirs('kl',exist_ok=True)
day_of=lambda t:datetime.datetime.utcfromtimestamp(t/1000).strftime('%Y-%m-%d')
need=sorted({(x['symbol'],day_of(t)) for x in r for t in (x['entry_ms']//60000*60000,x['entry_ms']//60000*60000+12*3600000)})
for sym,day in need:
    f=f'kl/{sym}-1m-{day}.zip'
    if os.path.exists(f) and os.path.getsize(f)>0: continue
    url=f'https://data.binance.vision/data/futures/um/daily/klines/{sym}/1m/{sym}-1m-{day}.zip'
    if subprocess.run(['curl','-s','-f','-m','60','-o',f,url]).returncode!=0: raise SystemExit(f'fetch failed: {sym} {day}')
cache={}
def day(sym,d):
    if (sym,d) not in cache:
        z=zipfile.ZipFile(f'kl/{sym}-1m-{d}.zip')
        cache[(sym,d)]=[[int(x[0]),float(x[1]),float(x[2]),float(x[3]),float(x[4])] for x in csv.reader(io.TextIOWrapper(z.open(z.namelist()[0]))) if x[0].isdigit()]
    return cache[(sym,d)]
used=set(); lev={}
for o in g['opens']:
    best=None
    for p in g['P']:
        if p['id'] in used or p['trader_id']!=o['trader'] or p['symbol']!=o['symbol'] or p['side']!=o['side']: continue
        dt=abs(p['entry_time']-o['t'])
        if dt<=180000 and (best is None or dt<best[0]): best=(dt,p)
    if best: used.add(best[1]['id']); lev[best[1]['id']]=o['lev']
T=[]
for x in r:
    a=x['entry_ms']//60000*60000; b=a+12*3600000
    bars=sorted(v for d in {day_of(a),day_of(b)} for v in day(x['symbol'],d) if a<=v[0]<=b)
    if len(bars)<2 or bars[0][0]!=a: raise SystemExit(f'path incomplete for position {x["pos"]}')
    t=dict(x); t['bars']=bars; t['gaps']=sum(1 for i in range(1,len(bars)) if bars[i][0]-bars[i-1][0]!=60000); T.append(t)
for t in T: t['dec_lev']=lev.get(t['pos'])
wrong=[t['pos'] for t in T if (t['side']=='LONG' and not t['stop']<t['entry']) or (t['side']=='SHORT' and not t['stop']>t['entry'])]
print('trades',len(T),'stop on the wrong side of entry (owner ruling (a): excluded in the run):',wrong)
json.dump(T,open('u0h2_trades.json','w'))
