package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TA-3: run state persisted across harness restarts.
//
// With --run-id the harness saves, after every completed round (accepted or
// contract-rejected), the native watch state, the Task-2 event store and
// the round counter to --state-file (default <out-dir>/state-<run-id>.json),
// atomically. --resume loads that file and continues with the next round;
// cycle numbers equal round numbers, so they continue too and are never
// reused (the Tier A restarts re-ran cycle 1 and duplicated 77 rows).
// Starting a run id whose state file already exists without --resume is
// refused for the same reason.

const harnessStateSchemaVersion = 1

type harnessState struct {
	SchemaVersion   int             `json:"schema_version"`
	RunID           string          `json:"run_id"`
	HarnessGitSHA   string          `json:"harness_git_sha"`
	PlannedRounds   int             `json:"planned_rounds"`
	LastRound       int             `json:"last_round"`        // last completed round (accepted or rejected)
	NextCycleNumber int             `json:"next_cycle_number"` // = LastRound + 1
	AcceptedRounds  int             `json:"accepted_rounds"`   // cumulative across restarts
	RejectedRounds  int             `json:"rejected_rounds"`   // cumulative across restarts
	WatchState      json.RawMessage `json:"watch_state"`
	T2State         json.RawMessage `json:"t2_state"`
	SavedAt         time.Time       `json:"saved_at"`
}

func defaultStateFile(outDir, runID string) string {
	return filepath.Join(outDir, "state-"+runID+".json")
}

// saveHarnessState writes the state atomically: temp file in the same
// directory, fsync, rename, fsync of the directory.
func saveHarnessState(path string, s harnessState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func loadHarnessState(path string) (harnessState, error) {
	var s harnessState
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("state file %s: %w", path, err)
	}
	if s.SchemaVersion != harnessStateSchemaVersion {
		return s, fmt.Errorf("state file %s: schema version %d, want %d", path, s.SchemaVersion, harnessStateSchemaVersion)
	}
	return s, nil
}

// prepareRunState validates the run-id / state-file / resume flags, loads
// the state when resuming, and returns the first round to run.
func prepareRunState(opts *validationOptions) (first int, prior harnessState, err error) {
	if opts.runID == "" {
		if opts.resume {
			return 0, prior, errors.New("--resume requires --run-id")
		}
		return 1, prior, nil
	}
	if opts.stateFile == "" {
		opts.stateFile = defaultStateFile(opts.outDir, opts.runID)
	}
	_, statErr := os.Stat(opts.stateFile)
	exists := statErr == nil
	if !opts.resume {
		if exists {
			return 0, prior, fmt.Errorf("state file %s already exists for run %q: use --resume, or a new --run-id (starting again would reuse cycle numbers)", opts.stateFile, opts.runID)
		}
		return 1, prior, nil
	}
	if !exists {
		return 0, prior, fmt.Errorf("--resume: state file %s not found", opts.stateFile)
	}
	prior, err = loadHarnessState(opts.stateFile)
	if err != nil {
		return 0, prior, err
	}
	if prior.RunID != opts.runID {
		return 0, prior, fmt.Errorf("--resume: state file %s belongs to run %q, not %q", opts.stateFile, prior.RunID, opts.runID)
	}
	if sha, _ := harnessBuildInfo(); prior.HarnessGitSHA != sha && !opts.resumeAllowSHAChange {
		return 0, prior, fmt.Errorf("--resume: state saved by harness %s, this binary is %s (pass --resume-allow-sha-change to accept)", prior.HarnessGitSHA, sha)
	}
	if prior.NextCycleNumber != prior.LastRound+1 {
		return 0, prior, fmt.Errorf("--resume: inconsistent state (last_round %d, next_cycle_number %d)", prior.LastRound, prior.NextCycleNumber)
	}
	if err := opts.watchState.ImportState(prior.WatchState); err != nil {
		return 0, prior, err
	}
	if err := opts.t2Store.LoadState(prior.T2State); err != nil {
		return 0, prior, err
	}
	return prior.NextCycleNumber, prior, nil
}

// snapshotHarnessState captures the committed state after a round.
func snapshotHarnessState(opts validationOptions, lastRound, accepted, rejected int) (harnessState, error) {
	ws, err := opts.watchState.ExportState()
	if err != nil {
		return harnessState{}, err
	}
	ts, err := opts.t2Store.MarshalState()
	if err != nil {
		return harnessState{}, err
	}
	sha, _ := harnessBuildInfo()
	return harnessState{
		SchemaVersion:   harnessStateSchemaVersion,
		RunID:           opts.runID,
		HarnessGitSHA:   sha,
		PlannedRounds:   opts.rounds,
		LastRound:       lastRound,
		NextCycleNumber: lastRound + 1,
		AcceptedRounds:  accepted,
		RejectedRounds:  rejected,
		WatchState:      ws,
		T2State:         ts,
		SavedAt:         time.Now().UTC(),
	}, nil
}
