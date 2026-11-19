package api

import (
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type cellsResponse struct {
	Source     string                     `json:"source"`
	Hysteresis string                     `json:"hysteresis"`
	Rule       store.V7CellRuleConfig     `json:"rule"`
	Tables     store.V7CellTables         `json:"tables"`
	Excluded   map[string]int             `json:"excluded_no_r"`
	Drift      []store.V7CalibrationDrift `json:"calibration_drift"`
}

func u12Server(t *testing.T, recs []store.HunterV7SignalRecord) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if len(recs) > 0 {
		if err := st.HunterV7Signal().CreateBatch(recs); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	return &Server{store: st}
}

func u12Get(t *testing.T, s *Server, query string) (int, cellsResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/hunter/v7/cells"+query, nil)
	s.handleHunterV7Cells(c)
	var resp cellsResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return w.Code, resp
}

// u12Records is a U5-tagged record set: three classes, several bands and
// regimes, all outcome sources, rows without a fee-net R, rows outside
// the window, and rows with expected_r in both drift windows.
func u12Records(now time.Time) []store.HunterV7SignalRecord {
	rng := rand.New(rand.NewSource(12))
	classes := []string{"TRIGGER", "IMPULSE", "REVERSAL", ""}
	bands := []string{"lt_1.0", "1.0_1.2", "1.2_1.5", "1.5_2.0", "ge_2.0"}
	regimes := []string{"trend_up", "chop", ""}
	sources := []string{store.V7OutcomeSourcePromptOpenReview, store.V7OutcomeSourcePromptOpenReview, store.V7OutcomeSourceWatchAudit, store.V7OutcomeSourceShadow}
	var out []store.HunterV7SignalRecord
	for i := 0; i < 400; i++ {
		age := time.Duration(rng.Intn(13*24)) * time.Hour // inside the 7d window or the previous one
		rec := store.HunterV7SignalRecord{
			CycleNumber: i, Timestamp: now.Add(-age - time.Minute), Symbol: "S", Direction: "LONG", SetupType: "x", Status: "candidate",
			SignalClass: classes[rng.Intn(len(classes))], RRBand: bands[rng.Intn(len(bands))], MarketRegime: regimes[rng.Intn(len(regimes))],
			OutcomeSource: sources[rng.Intn(len(sources))], CellStatus: store.V7CellStatusProbation,
		}
		switch k := rng.Intn(10); {
		case k < 4:
			r := 0.5 + rng.Float64()
			rec.FeeNetOutcome, rec.FeeNetR = store.V7FeeNetTP1, &r
		case k < 8:
			r := -1.0
			rec.FeeNetOutcome, rec.FeeNetR = store.V7FeeNetStop, &r
		case k < 9:
			r := 0.0
			rec.FeeNetOutcome, rec.FeeNetR = store.V7FeeNetNone, &r
		default:
			rec.FeeNetOutcome = store.V7FeeNetDegenerateStop
		}
		if rec.FeeNetR != nil && rng.Intn(2) == 0 {
			er := rng.Float64() - 0.3
			rec.ExpectedR, rec.CalibrationVersion = &er, "cal-a"
		}
		out = append(out, rec)
	}
	// Unresolved and stale rows never count.
	out = append(out, store.HunterV7SignalRecord{CycleNumber: 999, Timestamp: now.Add(-time.Hour), Symbol: "U", Direction: "LONG", SetupType: "x", Status: "candidate", SignalClass: "TRIGGER", OutcomeSource: store.V7OutcomeSourcePromptOpenReview})
	stale := -1.0
	out = append(out, store.HunterV7SignalRecord{CycleNumber: 998, Timestamp: now.AddDate(0, 0, -30), Symbol: "O", Direction: "LONG", SetupType: "x", Status: "candidate", SignalClass: "TRIGGER", OutcomeSource: store.V7OutcomeSourcePromptOpenReview, FeeNetOutcome: store.V7FeeNetStop, FeeNetR: &stale})
	return out
}

type u12Agg struct {
	n, tp1, stop, none int
	sum                float64
}

// u12GroupBy is the reference: a direct group-by over the records.
func u12GroupBy(recs []store.HunterV7SignalRecord, from time.Time, source string, key func(store.HunterV7SignalRecord) string) map[string]*u12Agg {
	out := map[string]*u12Agg{}
	for _, r := range recs {
		if r.FeeNetR == nil || r.Timestamp.Before(from) || (source != "all" && r.OutcomeSource != source) {
			continue
		}
		k := key(r)
		a := out[k]
		if a == nil {
			a = &u12Agg{}
			out[k] = a
		}
		a.n++
		a.sum += *r.FeeNetR
		switch r.FeeNetOutcome {
		case store.V7FeeNetTP1:
			a.tp1++
		case store.V7FeeNetStop:
			a.stop++
		case store.V7FeeNetNone:
			a.none++
		}
	}
	return out
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func u12Compare(t *testing.T, label string, rows []store.V7CellRow, ref map[string]*u12Agg, key func(store.V7CellRow) string) {
	t.Helper()
	if len(rows) != len(ref) {
		t.Fatalf("%s: %d rows, reference %d", label, len(rows), len(ref))
	}
	for _, row := range rows {
		a := ref[key(row)]
		if a == nil || row.N != a.n || row.TP1 != a.tp1 || row.Stop != a.stop || row.None != a.none ||
			math.Abs(row.MeanR-a.sum/float64(a.n)) > 1e-9 || math.Abs(row.TP1Rate-float64(a.tp1)/float64(a.n)) > 1e-12 {
			t.Fatalf("%s %q: %+v vs reference %+v", label, key(row), row, a)
		}
		if row.CILo > row.MeanR+1e-9 || row.CIHi < row.MeanR-1e-9 {
			t.Fatalf("%s %q: CI [%v, %v] excludes the mean %v", label, key(row), row.CILo, row.CIHi, row.MeanR)
		}
	}
}

// U12 acceptance: the per-class, per-cell and per cell × regime tables
// match a direct group-by over the U5 records, for every source filter.
func TestU12CellsMatchU5Records(t *testing.T) {
	now := time.Now().UTC()
	recs := u12Records(now)
	s := u12Server(t, recs)
	from := now.AddDate(0, 0, -7)
	classKey := func(r store.HunterV7SignalRecord) string { return orDefault(r.SignalClass, store.V7ClassUnassigned) }
	cellKey := func(r store.HunterV7SignalRecord) string {
		return classKey(r) + "|" + orDefault(r.RRBand, store.V7BandUnknown)
	}
	regimeKey := func(r store.HunterV7SignalRecord) string {
		return cellKey(r) + "|" + orDefault(r.MarketRegime, store.V7CellRegimeUnknown)
	}
	for _, src := range []string{"", "all", store.V7OutcomeSourcePromptOpenReview, store.V7OutcomeSourceWatchAudit, store.V7OutcomeSourceShadow} {
		q := "?days=7"
		want := src
		if src != "" {
			q += "&source=" + src
		} else {
			want = store.V7OutcomeSourcePromptOpenReview // default (owner decision 3)
		}
		code, resp := u12Get(t, s, q)
		if code != http.StatusOK || resp.Source != want || resp.Hysteresis != "none" {
			t.Fatalf("%q: code %d source %q hysteresis %q", src, code, resp.Source, resp.Hysteresis)
		}
		u12Compare(t, want+" classes", resp.Tables.Classes, u12GroupBy(recs, from, want, classKey),
			func(r store.V7CellRow) string { return r.Class })
		u12Compare(t, want+" cells", resp.Tables.Cells, u12GroupBy(recs, from, want, cellKey),
			func(r store.V7CellRow) string { return r.Class + "|" + r.RRBand })
		u12Compare(t, want+" cell_regimes", resp.Tables.CellRegimes, u12GroupBy(recs, from, want, regimeKey),
			func(r store.V7CellRow) string { return r.Class + "|" + r.RRBand + "|" + r.Regime })
		total := 0
		for _, a := range u12GroupBy(recs, from, want, classKey) {
			total += a.n
		}
		if resp.Tables.Rows != total {
			t.Fatalf("%s rows %d vs %d", want, resp.Tables.Rows, total)
		}
		for _, row := range resp.Tables.Cells {
			if exp := store.V7EvaluateCellStatus(row.N, row.CILo, row.CIHi, true, resp.Rule.MinSamples); row.EvaluatedStatus != exp {
				t.Fatalf("evaluated status %s vs rule %s", row.EvaluatedStatus, exp)
			}
			if len(row.StampedStatus) != 1 || row.StampedStatus[0] != store.V7CellStatusProbation {
				t.Fatalf("stamped = %v", row.StampedStatus)
			}
		}
	}
	// Defaults reach the response when no strategy config exists.
	_, resp := u12Get(t, s, "")
	if resp.Rule.MinSamples != 30 || resp.Rule.CILevel != 0.90 {
		t.Fatalf("rule = %+v", resp.Rule)
	}
	if resp.Excluded[store.V7FeeNetDegenerateStop] == 0 {
		t.Fatal("degenerate rows must be reported as excluded")
	}
	// Drift: one version, both windows populated.
	if len(resp.Drift) != 1 || resp.Drift[0].Version != "cal-a" || resp.Drift[0].Current.N == 0 || resp.Drift[0].Previous.N == 0 {
		t.Fatalf("drift = %+v", resp.Drift)
	}
	nCur := 0
	for _, r := range recs {
		if r.ExpectedR != nil && r.FeeNetR != nil && !r.Timestamp.Before(from) && r.OutcomeSource == store.V7OutcomeSourcePromptOpenReview {
			nCur++
		}
	}
	if resp.Drift[0].Current.N != nCur {
		t.Fatalf("drift current n %d vs %d", resp.Drift[0].Current.N, nCur)
	}
}

func TestU12CellsRejectsUnknownSource(t *testing.T) {
	s := u12Server(t, nil)
	if code, _ := u12Get(t, s, "?source=bogus"); code != http.StatusBadRequest {
		t.Fatalf("code = %d", code)
	}
	code, resp := u12Get(t, s, "?days=500")
	if code != http.StatusOK || resp.Tables.Classes == nil || resp.Drift == nil {
		t.Fatalf("empty store: code %d %+v", code, resp)
	}
}
