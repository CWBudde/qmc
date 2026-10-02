package qmc_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/cwbudde/qmc"
)

func TestConcurrentIndexedAccessPreservesCursor(t *testing.T) {
	makeHalton := func(d int, o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewHalton(d, o...) }
	makeSobol := func(d int, o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewSobol(d, o...) }

	cases := []struct {
		name string
		dims int
		new  func(int, ...qmc.Option) (qmc.Sequence, error)
		opts []qmc.Option
	}{
		{"halton/plain", 3, makeHalton, nil},
		{"halton/skip", 3, makeHalton, []qmc.Option{qmc.WithSkip(64)}},
		{"halton/leap", 3, makeHalton, []qmc.Option{qmc.WithLeap(7)}},
		{"halton/digit", 3, makeHalton, []qmc.Option{qmc.WithScrambling(29)}},
		{"halton/nested", 3, makeHalton, []qmc.Option{qmc.WithNestedScrambling(29)}},
		{"halton/digit/skip/leap", 3, makeHalton, []qmc.Option{qmc.WithScrambling(29), qmc.WithSkip(64), qmc.WithLeap(7)}},
		{"halton/nested/skip/leap", 3, makeHalton, []qmc.Option{qmc.WithNestedScrambling(29), qmc.WithSkip(64), qmc.WithLeap(7)}},
		{"halton/nested/scratch", 100, makeHalton, []qmc.Option{qmc.WithNestedScrambling(29)}},
		{"sobol/plain", 3, makeSobol, nil},
		{"sobol/skip", 3, makeSobol, []qmc.Option{qmc.WithSkip(64)}},
		{"sobol/leap", 3, makeSobol, []qmc.Option{qmc.WithLeap(7)}},
		{"sobol/shift", 3, makeSobol, []qmc.Option{qmc.WithDigitalShift(29)}},
		{"sobol/owen", 3, makeSobol, []qmc.Option{qmc.WithOwenScrambling(29)}},
		{"sobol/shift/skip/leap", 3, makeSobol, []qmc.Option{qmc.WithDigitalShift(29), qmc.WithSkip(64), qmc.WithLeap(7)}},
		{"sobol/owen/skip/leap", 3, makeSobol, []qmc.Option{qmc.WithOwenScrambling(29), qmc.WithSkip(64), qmc.WithLeap(7)}},
		{"sobol/custom", 3, makeSobol, []qmc.Option{qmc.WithDirectionNumbers(strings.NewReader("d s a m_i\n2 1 0 1\n3 2 1 1 3\n")), qmc.WithSkip(7), qmc.WithLeap(7), qmc.WithOwenScrambling(29)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := tc.new(tc.dims, tc.opts...)
			if err != nil {
				t.Fatal(err)
			}

			const points = 128

			reference := make([][]float64, points)
			for i := range reference {
				reference[i] = g.At(i * 17)
			}

			g.Next() // consume one point before concurrent indexed access

			start := make(chan struct{})

			var group sync.WaitGroup
			for worker := range 8 {
				group.Add(1)
				go func() {
					defer group.Done()

					<-start

					dst := make([]float64, g.Dims()+1)
					dst[g.Dims()] = -7 // indexed calls must leave spare capacity alone

					for j := range points {
						i := (j*31 + worker*13) % points
						g.AtInto(i*17, dst)

						if !reflect.DeepEqual(dst[:g.Dims()], reference[i]) || dst[g.Dims()] != -7 {
							t.Errorf("worker %d: AtInto(%d) differs from sequential reference", worker, i*17)
							return
						}

						if got := g.At(i * 17); !reflect.DeepEqual(got, reference[i]) {
							t.Errorf("worker %d: At(%d) differs from sequential reference", worker, i*17)
							return
						}
					}
				}()
			}

			close(start)
			group.Wait()

			if got, want := g.Next(), g.At(1); !reflect.DeepEqual(got, want) {
				t.Fatalf("indexed calls changed cursor: Next=%v, At(1)=%v", got, want)
			}
		})
	}
}
