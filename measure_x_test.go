package qmc_test

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/qmc"
)

func TestMeasureReport(t *testing.T) {
	rng := rand.New(rand.NewSource(20240901))
	const streams = 10

	fmt.Println("== 1. star discrepancy, scrambled Halton vs math/rand ==")
	fmt.Printf("%4s %6s %12s %12s %8s\n", "s", "N", "halton", "random", "ratio")
	for _, s := range []int{1, 2, 3, 4} {
		ns := []int{64, 256, 512}
		if s == 4 {
			ns = []int{64, 128, 160}
		}
		for _, n := range ns {
			q, m := 0.0, 0.0
			for seed := 1; seed <= streams; seed++ {
				g, _ := qmc.NewHalton(s, qmc.WithSkip(64), qmc.WithScrambling(uint64(seed)))
				a, err := qmc.StarDiscrepancy(qmc.Draw(g, n))
				if err != nil { t.Fatal(err) }
				b, err := qmc.StarDiscrepancy(randomPoints(rng, n, s))
				if err != nil { t.Fatal(err) }
				q += a; m += b
			}
			q /= streams; m /= streams
			fmt.Printf("%4d %6d %12.6f %12.6f %8.2f\n", s, n, q, m, m/q)
		}
	}

	fmt.Println("\n== 2. CD2 saturation table, N=1024 ==")
	fmt.Printf("%4s %12s %12s %12s %8s %14s\n", "s", "cd2(qmc)", "cd2(rand)", "analytic", "ratio", "diag share")
	for _, s := range []int{2, 5, 10, 15, 20, 30, 39} {
		q, m := 0.0, 0.0
		const n = 1024
		for seed := 1; seed <= streams; seed++ {
			g, _ := qmc.NewHalton(s, qmc.WithSkip(64), qmc.WithScrambling(uint64(seed)))
			a, err := qmc.CenteredL2Discrepancy(qmc.Draw(g, n))
			if err != nil { t.Fatal(err) }
			b, err := qmc.CenteredL2Discrepancy(randomPoints(rng, n, s))
			if err != nil { t.Fatal(err) }
			q += a; m += b
		}
		q /= streams; m /= streams
		sf := float64(s)
		analyticSq := (math.Pow(1.25, sf) - math.Pow(13.0/12.0, sf)) / n
		diag := math.Pow(1.25, sf) / n
		fmt.Printf("%4d %12.6f %12.6f %12.6f %8.3f %13.1f%%\n",
			s, q, m, math.Sqrt(analyticSq), m/q, 100*diag/analyticSq)
	}

	fmt.Println("\n== 3. RMS integration error over the same 39d/1024 sets ==")
	{
		const dims, n = 39, 1024
		qs, ms := 0.0, 0.0
		for seed := 1; seed <= streams; seed++ {
			g, _ := qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithScrambling(uint64(seed)))
			qe := meanProductIntegrand(qmc.Draw(g, n)) - 1
			me := meanProductIntegrand(randomPoints(rng, n, dims)) - 1
			qs += qe * qe
			ms += me * me
		}
		qr, mr := math.Sqrt(qs/streams), math.Sqrt(ms/streams)
		fmt.Printf("qmc RMS %.4e, mc RMS %.4e, ratio %.1fx\n", qr, mr, mr/qr)
	}
}
