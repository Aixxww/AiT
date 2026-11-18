"""Pure exit walks used by U0-F2 and U0-H2 (no I/O, unit-tested).

A prepared trade is a dict: E (entry), d (risk distance), sg (+1 long /
-1 short), tp (exchange take-profit, in R), fast (bool), and B, a list of
1m bars as (favourable extreme R, adverse extreme R, close R, open_ms)
starting at the fill bar. Fees: taker 0.05% per side on every leg.

V2 and Pw are byte-for-byte the functions of the frozen U0-H2 run
(scripts/t2_u0/u0h2_run.py); test_walks.py checks that.
"""
FEE=0.0005
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


def V1(f, pct):  # Nofx literal: static SL+TP; full close at a close when price move >5% and giveback >=40% of peak
    peak = None
    for fav, adv, cl, t in f['B']:
        if adv <= -1:
            return done(f, -1)
        if fav >= f['tp']:
            return done(f, f['tp'])
        peak = cl if peak is None else max(peak, cl)
        dd = (peak - cl) / peak * 100 if peak > 0 and cl < peak else 0
        if pct(cl) > 5.0 and dd >= 40:
            return done(f, cl)
    return done(f, f['B'][-1][2])


def V3(f):  # Hummingbot triple barrier: SL, TP, time limit 60/240 min; trailing on fee-net PnL, activation 1.0R, delta 0.5R
    trig = None
    t0 = f['B'][0][3]
    for fav, adv, cl, t in f['B']:
        if adv <= -1:
            return done(f, -1)
        if fav >= f['tp']:
            return done(f, f['tp'])
        net = done(f, cl)
        if trig is None:
            if net > 1.0:
                trig = net - 0.5
        else:
            if net < trig:
                return net
            if net - 0.5 > trig:
                trig = net - 0.5
        if (t - t0) / 60000 >= (60 if f['fast'] else 240):
            return net
    return done(f, f['B'][-1][2])
