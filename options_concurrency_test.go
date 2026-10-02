package qmc_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/cwbudde/qmc"
)

func TestOptionsAreReusableAcrossConcurrentConstructors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		shared    []qmc.Option
		canonical []qmc.Option
	}{
		{"negative skip", []qmc.Option{qmc.WithSkip(-1)}, []qmc.Option{qmc.WithSkip(0)}},
		{"zero leap", []qmc.Option{qmc.WithLeap(0)}, []qmc.Option{qmc.WithLeap(1)}},
		{"both clamped", []qmc.Option{qmc.WithSkip(-1), qmc.WithLeap(0)}, nil},
		{"valid", []qmc.Option{qmc.WithSkip(11), qmc.WithLeap(5)}, []qmc.Option{qmc.WithSkip(11), qmc.WithLeap(5)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := qmc.NewHalton(2, tc.canonical...)
			if err != nil {
				t.Fatal(err)
			}

			s, err := qmc.NewSobol(2, tc.canonical...)
			if err != nil {
				t.Fatal(err)
			}

			wantH, wantS := h.At(3), s.At(3)
			start := make(chan struct{})

			var group sync.WaitGroup
			for range 64 {
				group.Add(1)
				go func() {
					defer group.Done()

					<-start

					h, err := qmc.NewHalton(2, tc.shared...)
					if err != nil {
						t.Error(err)
						return
					}

					s, err := qmc.NewSobol(2, tc.shared...)
					if err != nil {
						t.Error(err)
						return
					}

					if got := h.At(3); !reflect.DeepEqual(got, wantH) {
						t.Errorf("Halton: got %v, want %v", got, wantH)
					}

					if got := s.At(3); !reflect.DeepEqual(got, wantS) {
						t.Errorf("Sobol: got %v, want %v", got, wantS)
					}
				}()
			}

			close(start)
			group.Wait()
		})
	}
}
