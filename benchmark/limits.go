package benchmark

import (
	"fmt"
	"sort"
	"time"
)

type Measurement struct {
	Name     string
	Duration time.Duration
}

type Run struct {
	Measurements            []Measurement
	PhaseMeasurements       []Measurement
	ResolverMeasurements    []Measurement
	SQLiteWriteMeasurements []Measurement
	PeakRSSBytes            uint64
	DatabaseBytes           int64
	OutputChecksum          string
}

var defaultApprovedLimits = map[string]time.Duration{
	"initial_index":      60 * time.Second,
	"incremental_update": 2 * time.Second,
	"query":              500 * time.Millisecond,
	"path":               500 * time.Millisecond,
	"explain":            500 * time.Millisecond,
}

// approvedLimitsBySourceFiles holds acceptance limits calibrated for one exact-scale
// benchmark corpus. A corpus size without an entry falls back to defaultApprovedLimits.
// A profile may omit a measurement name that has no limit calibrated at that scale yet;
// Validate then skips that measurement instead of failing the run.
var approvedLimitsBySourceFiles = map[int]map[string]time.Duration{
	1000: {
		"initial_index":      10 * time.Second,
		"incremental_update": 2 * time.Second,
		"query":              500 * time.Millisecond,
		"path":               500 * time.Millisecond,
		"explain":            500 * time.Millisecond,
	},
	// query, path, and explain scale with corpus size and have no calibrated limit here yet.
	10000: {
		"initial_index": 100 * time.Second,
	},
}

func approvedLimitsFor(sourceFiles int) map[string]time.Duration {
	if limits, ok := approvedLimitsBySourceFiles[sourceFiles]; ok {
		return limits
	}
	return defaultApprovedLimits
}

var measurementOrder = []string{
	"initial_index",
	"incremental_update",
	"query",
	"path",
	"explain",
}

var phaseMeasurementOrder = []string{
	"extraction",
	"extraction_write_overlap",
	"resolution",
	"publication_preparation",
	"sqlite_write",
	"commit",
	"staged_transaction",
}

var resolverMeasurementOrder = []string{
	"affected_source_selection",
	"contribution_restoration",
	"workspace_resolution",
}

var sqliteWriteMeasurementOrder = []string{
	"workspace_nodes",
	"workspace_edges",
	"file_contributions",
	"contribution_nodes",
	"contribution_edges",
	"contribution_extensions",
	"contribution_dependencies",
	"contribution_exported_surfaces",
	"contribution_diagnostics",
	"contribution_unresolved_references",
	"contribution_module_bindings",
	"contribution_symbol_references",
}

const (
	requiredRuns = 1
)

func Validate(runs []Run, sourceFiles int) error {
	if len(runs) != requiredRuns {
		return fmt.Errorf("benchmark requires %d runs, got %d", requiredRuns, len(runs))
	}
	limits := approvedLimitsFor(sourceFiles)

	durations := make(map[string][]time.Duration, len(limits))
	for _, run := range runs {
		seen := make(map[string]bool, len(limits))
		for _, measurement := range run.Measurements {
			// A measurement outside this corpus size's profile has no calibrated limit; skip it.
			if _, tracked := limits[measurement.Name]; !tracked {
				continue
			}
			if seen[measurement.Name] {
				return fmt.Errorf("benchmark run contains duplicate measurement %q", measurement.Name)
			}
			seen[measurement.Name] = true
			durations[measurement.Name] = append(durations[measurement.Name], measurement.Duration)
		}
		for name := range limits {
			if !seen[name] {
				return fmt.Errorf("benchmark run is missing measurement %q", name)
			}
		}
	}

	for name, limit := range limits {
		values := durations[name]
		sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
		median := values[len(values)/2]
		if median > limit {
			return fmt.Errorf("benchmark %q median %s exceeds approved limit %s", name, median, limit)
		}
	}
	return nil
}

func ValidateReport(runs []Run, sourceFiles int) error {
	if err := Validate(runs, sourceFiles); err != nil {
		return err
	}
	checksum := ""
	for _, run := range runs {
		if err := validateMeasurementOrder(run.PhaseMeasurements, phaseMeasurementOrder); err != nil {
			return fmt.Errorf("benchmark run phase measurements: %w", err)
		}
		if err := validateMeasurementOrder(run.ResolverMeasurements, resolverMeasurementOrder); err != nil {
			return fmt.Errorf("benchmark run resolver measurements: %w", err)
		}
		if err := validateMeasurementOrder(run.SQLiteWriteMeasurements, sqliteWriteMeasurementOrder); err != nil {
			return fmt.Errorf("benchmark run SQLite write measurements: %w", err)
		}
		if run.PeakRSSBytes == 0 || run.DatabaseBytes <= 0 || run.OutputChecksum == "" {
			return fmt.Errorf("benchmark run is missing peak RSS, database size, or output checksum")
		}
		if checksum == "" {
			checksum = run.OutputChecksum
		} else if run.OutputChecksum != checksum {
			return fmt.Errorf("benchmark runs have different output checksums")
		}
	}
	return nil
}

func Medians(runs []Run) []Measurement {
	return medians(runs, measurementOrder, func(run Run) []Measurement { return run.Measurements })
}

func PhaseMedians(runs []Run) []Measurement {
	return medians(runs, phaseMeasurementOrder, func(run Run) []Measurement { return run.PhaseMeasurements })
}

func ResolverMedians(runs []Run) []Measurement {
	return medians(runs, resolverMeasurementOrder, func(run Run) []Measurement { return run.ResolverMeasurements })
}

func SQLiteWriteMedians(runs []Run) []Measurement {
	return medians(runs, sqliteWriteMeasurementOrder, func(run Run) []Measurement { return run.SQLiteWriteMeasurements })
}

func OrderSQLiteWriteMeasurements(measurements []Measurement) []Measurement {
	durations := make(map[string]time.Duration, len(measurements))
	for _, measurement := range measurements {
		durations[measurement.Name] += measurement.Duration
	}
	ordered := make([]Measurement, 0, len(sqliteWriteMeasurementOrder))
	for _, name := range sqliteWriteMeasurementOrder {
		ordered = append(ordered, Measurement{Name: name, Duration: durations[name]})
	}
	return ordered
}

func validateMeasurementOrder(measurements []Measurement, order []string) error {
	if len(measurements) != len(order) {
		return fmt.Errorf("got %d, want %d", len(measurements), len(order))
	}
	for index, name := range order {
		if measurements[index].Name != name {
			return fmt.Errorf("measurement %d is %q, want %q", index, measurements[index].Name, name)
		}
	}
	return nil
}

func medians(runs []Run, order []string, selectMeasurements func(Run) []Measurement) []Measurement {
	durations := make(map[string][]time.Duration, len(order))
	for _, run := range runs {
		for _, measurement := range selectMeasurements(run) {
			durations[measurement.Name] = append(durations[measurement.Name], measurement.Duration)
		}
	}

	measurements := make([]Measurement, 0, len(order))
	for _, name := range order {
		values := durations[name]
		sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
		measurements = append(measurements, Measurement{Name: name, Duration: values[len(values)/2]})
	}
	return measurements
}
