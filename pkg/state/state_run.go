package state

import (
	"fmt"
	"sync"

	"github.com/roboll/helmfile/pkg/helmexec"
)

type result struct {
	release ReleaseSpec
	err     error
}

func (st *HelmState) scatterGather(concurrency int, items int, produceInputs func(), receiveInputsAndProduceIntermediates func(int), aggregateIntermediates func()) {

	if concurrency < 1 || concurrency > items {
		concurrency = items
	}

	for _, r := range st.Releases {
		if r.Tillerless != nil {
			if *r.Tillerless {
				concurrency = 1
			}
		} else if st.HelmDefaults.Tillerless {
			concurrency = 1
		}
	}

	// WaitGroup is required to wait until goroutine per job in job queue cleanly stops.
	var waitGroup sync.WaitGroup
	waitGroup.Add(concurrency)

	go produceInputs()

	for w := 1; w <= concurrency; w++ {
		go func(id int) {
			st.logger.Debugf("worker %d/%d started", id, concurrency)
			receiveInputsAndProduceIntermediates(id)
			st.logger.Debugf("worker %d/%d finished", id, concurrency)
			waitGroup.Done()
		}(w)
	}

	aggregateIntermediates()

	// Wait until all the goroutines to gracefully finish
	waitGroup.Wait()
}

func (st *HelmState) scatterGatherReleases(helm helmexec.Interface, concurrency int,
	do func(ReleaseSpec, int) error) []error {
	var errs []error

	inputs := st.Releases
	inputsSize := len(inputs)

	releases := make(chan ReleaseSpec)
	results := make(chan result)

	st.scatterGather(
		concurrency,
		inputsSize,
		func() {
			for _, release := range inputs {
				releases <- release
			}
			close(releases)
		},
		func(id int) {
			for release := range releases {
				err := do(release, id)
				st.logger.Debugf("sending result for release: %s\n", release.Name)
				results <- result{release: release, err: err}
				st.logger.Debugf("sent result for release: %s\n", release.Name)
			}
		},
		func() {
			for i := range inputs {
				st.logger.Debugf("receiving result %d", i)
				r := <-results
				if r.err != nil {
					errs = append(errs, fmt.Errorf("release \"%s\" failed: %v", r.release.Name, r.err))
				} else {
					st.logger.Debugf("received result for release \"%s\"", r.release.Name)
				}
				st.logger.Debugf("received result for %d", i)
			}
		},
	)

	if len(errs) != 0 {
		return errs
	}

	return nil
}
