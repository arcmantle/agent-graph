package index

import (
	"context"
	"runtime"
	"testing"

	"agent-graph/extractor"
	"agent-graph/extractors/registry"
	"agent-graph/extractors/typescript"
	"agent-graph/workspace"
)

type blockingFirstContributionWriteSession struct {
	acceptingContributionSession
	writeStarted chan struct{}
	releaseWrite chan struct{}
	blocked      bool
}

func (session *blockingFirstContributionWriteSession) WriteContribution(ctx context.Context, contribution extractor.Contribution) error {
	if !session.blocked {
		session.blocked = true
		close(session.writeStarted)
		select {
		case <-session.releaseWrite:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestInitialIndexPipelineMeasuresBoundedQueuePressureAndDirectOverlap(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(64)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })

	registered, err := registry.Default()
	if err != nil {
		t.Fatalf("create extractor registry: %v", err)
	}
	sourceCount := 64
	sources := make([]workspace.Source, 0, sourceCount)
	for sourceIndex := 0; sourceIndex < sourceCount; sourceIndex++ {
		sources = append(sources, workspace.Source{Path: "source-" + string(rune('A'+sourceIndex)) + ".ts"})
	}
	pipeline := &InitialIndexPipeline{
		sources:    sources,
		registered: registered,
		metrics: initialPipelineMetrics{
			queueCapacity: initialContributionQueueCapacity,
		},
	}
	session := &blockingFirstContributionWriteSession{
		writeStarted: make(chan struct{}),
		releaseWrite: make(chan struct{}),
	}
	extracted := make(chan struct{}, sourceCount)
	completed := make(chan error, 1)
	go func() {
		_, _, _, err := pipeline.extractAndWriteContributions(
			context.Background(),
			session,
			func(_ string, source workspace.Source, registered registry.Registry, _ *typescript.Worker) (extractedSource, error) {
				contribution, err := emptyContribution(source.Path, registered)
				extracted <- struct{}{}
				return extractedSource{contribution: contribution}, err
			},
			func() (*typescript.Worker, error) { return &typescript.Worker{}, nil },
			nil,
		)
		completed <- err
	}()

	<-session.writeStarted
	for range sourceCount {
		<-extracted
	}
	close(session.releaseWrite)
	if err := <-completed; err != nil {
		t.Fatalf("extract and write contributions: %v", err)
	}

	if pipeline.metrics.queueHighWater != initialContributionQueueCapacity {
		t.Errorf("queue high-water = %d, want capacity %d", pipeline.metrics.queueHighWater, initialContributionQueueCapacity)
	}
	if pipeline.metrics.producerBlocked <= 0 {
		t.Errorf("producer blocked duration = %s, want positive", pipeline.metrics.producerBlocked)
	}
	if pipeline.metrics.overlap <= 0 {
		t.Errorf("extraction/write overlap = %s, want positive", pipeline.metrics.overlap)
	}
}
