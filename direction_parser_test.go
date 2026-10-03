package qmc

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
)

func TestDirectionParserRejectsMalformedHeadersAndDegreeOneCoefficients(t *testing.T) {
	for _, input := range []string{
		"d s a wrong\n2 1 0 1\n",
		"not a header\n2 1 0 1\n",
		"d s a m_i extra\n2 1 0 1\n",
		"2 1 1 1\n",
		"2 1 9223372036854775808 1\n",
		"2 1 18446744073709551616 1\n",
		"2 1 0 1\n3 2 1 1 3 extra\n",
		"2 1 0 1\n4 2 1 1 3\n",
	} {
		if _, err := parseDirectionNumbers(strings.NewReader(input)); err == nil {
			t.Errorf("accepted malformed table %q", input)
		}
	}
}

func TestDirectionParserAcceptsOptionalHeaderAndBlankLines(t *testing.T) {
	for _, input := range []string{
		"2 1 0 1\n3 2 1 1 3", // final row without a newline
		"d s a m_i\n2 1 0 1\n\n3 2 1 1 3\n",
		"\n\n d\ts\ta\tm_i\n2 1 0 1\n3 2 1 1 3\n",
	} {
		rows, err := parseDirectionNumbers(strings.NewReader(input))
		if err != nil || len(rows) != 2 {
			t.Errorf("valid table %q: %d rows, %v", input, len(rows), err)
		}
	}
}

func TestDirectionParserPropagatesReaderFailure(t *testing.T) {
	want := errors.New("direction reader failed")
	r := io.MultiReader(strings.NewReader("d s a m_i\n2 1 0 1\n"), iotest.ErrReader(want))

	rows, err := parseDirectionNumbers(r)
	if !errors.Is(err, want) || rows != nil {
		t.Fatalf("reader error returned %v, %v; want nil rows and wrapped failure", rows, err)
	}
}

func TestDirectionRowCoefficientAndDegreeLimits(t *testing.T) {
	for _, degree := range []int{1, 2, 32} {
		bound := uint64(1) << uint(degree-1)

		fields := []string{"2", strconv.Itoa(degree), strconv.FormatUint(bound-1, 10)}
		for range degree {
			fields = append(fields, "1")
		}

		if _, err := parseDirectionRow(fields); err != nil {
			t.Fatalf("degree %d coefficient just below bound: %v", degree, err)
		}

		fields[2] = strconv.FormatUint(bound, 10)
		if _, err := parseDirectionRow(fields); err == nil {
			t.Errorf("degree %d coefficient at bound accepted", degree)
		}
	}

	for _, degree := range []string{"-1", "0", "33"} {
		if _, err := parseDirectionRow([]string{"2", degree, "0", "1"}); err == nil {
			t.Errorf("invalid degree %s accepted", degree)
		}
	}
}

func FuzzDirectionNumbersParser(f *testing.F) {
	for _, seed := range []string{"d s a m_i\n2 1 0 1\n3 2 1 1 3\n", "2 1 1 1\n", "d s a bad\n", "", "2 32 2147483648 1\n"} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 || bytes.Count(data, []byte{'\n'}) > 64 {
			t.Skip("bounded table-parser fuzz workload")
		}

		rows, err := parseDirectionNumbers(bytes.NewReader(data))
		if err != nil {
			return
		}

		g, err := NewSobol(min(len(rows)+1, 4), WithDirectionNumbers(bytes.NewReader(data)))
		if err != nil {
			t.Fatalf("parsed table cannot construct a generator: %v", err)
		}

		for _, x := range g.At(7) {
			if !(x >= 0 && x < 1) {
				t.Fatalf("parsed table produced out-of-range coordinate %g", x)
			}
		}
	})
}
