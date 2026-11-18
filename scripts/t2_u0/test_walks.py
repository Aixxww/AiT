"""Data-free tests for walks.py: synthetic paths, plus a check that V2 and
Pw are identical to the functions of the frozen U0-H2 run."""
import ast, os, unittest
import walks

def trade(B, tp=2.0, E=100.0, d=2.0, sg=1, fast=False):
    return dict(E=E, d=d, sg=sg, tp=tp, B=B, fast=fast)

def feeR(E, X, d):  # round trip, 0.05% per side
    return 0.0005 * (E + X) / d

class Walks(unittest.TestCase):
    def test_v2_stop_before_arming(self):
        f = trade([(0.3, -0.2, 0.1, 0), (0.4, -1.2, -1.1, 60000)])
        self.assertAlmostEqual(walks.V2(f), -1 - feeR(100, 98, 2))

    def test_v2_trails_after_offset(self):
        # peak 1.4R arms the stop at +0.9R; the next bar trades through it.
        f = trade([(1.4, 0.2, 1.3, 0), (1.3, 0.8, 0.9, 60000)], tp=3.0)
        self.assertAlmostEqual(walks.V2(f), 0.9 - feeR(100, 101.8, 2))

    def test_v2_take_profit(self):
        f = trade([(0.5, -0.1, 0.4, 0), (2.1, 0.3, 2.0, 60000)])
        self.assertAlmostEqual(walks.V2(f), 2.0 - feeR(100, 104, 2))

    def test_v2_stop_applies_from_next_bar(self):
        # the arming bar's own low (0.7 < 0.9) does not stop it out.
        f = trade([(1.4, 0.7, 1.2, 0)], tp=3.0)
        self.assertAlmostEqual(walks.V2(f), 1.2 - feeR(100, 102.4, 2))

    def test_plan_holds_through_dip(self):
        f = trade([(1.4, 0.2, 1.3, 0), (1.3, 0.1, 0.5, 60000), (2.1, 0.4, 2.0, 120000)])
        self.assertAlmostEqual(walks.Pw(f), 2.0 - feeR(100, 104, 2))

    def test_v3_time_limit(self):
        B = [(0.2, -0.1, 0.1, k * 60000) for k in range(62)]
        f = trade(B, fast=True)
        self.assertAlmostEqual(walks.V3(f), walks.done(f, 0.1))

    def test_frozen_functions_identical(self):
        here = os.path.dirname(os.path.abspath(__file__))
        def funcs(path):
            tree = ast.parse(open(path).read())
            return {n.name: ast.dump(n) for n in tree.body if isinstance(n, ast.FunctionDef)}
        frozen, live = funcs(os.path.join(here, 'u0h2_run.py')), funcs(os.path.join(here, 'walks.py'))
        for name in ('fee', 'done', 'V2', 'Pw'):
            self.assertEqual(frozen[name], live[name], name)

if __name__ == '__main__':
    unittest.main()
