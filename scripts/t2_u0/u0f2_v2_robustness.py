from u0f2_variants import *
def V2g(f,off,trail,close_only=False):
    stop=-1.0; peak=0.0
    for fav,adv,cl,t in f['B']:
        if adv<=stop: return done(f,stop)
        if fav>=f['tp']: return done(f,f['tp'])
        peak=max(peak,cl if close_only else fav)
        if peak>=off: stop=max(stop,peak-trail)
    return done(f,f['B'][-1][2])
print('\n=== V2 robustness (not part of the gate; neighbours of the pre-registered 1.0R/0.5R) ===')
for cname in ['C0 all 380 (U0-F)','C2 registry-tradable families (U6a/OD-27/28)','C3 drop U0-A score bands with CI<0']:
    s=[x for x in F if CUTS[cname](x)]
    row=[]
    for off,tr,co in [(1.0,0.5,True),(0.8,0.4,False),(1.2,0.6,False),(1.0,0.7,False),(1.5,0.5,False)]:
        dA=[V2g(P[x['index']],off,tr,co)-x['a_r'] for x in s]; p,lo,hi=boot_p(dA)
        row.append(f'{off}/{tr}{"c" if co else ""}: {sum(dA)/len(dA):+.3f} P={p:.2f}')
    print(f'  {cname[:24]:24s} n={len(s)}  '+' | '.join(row))
