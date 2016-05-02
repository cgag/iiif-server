package main

import "testing"

func TestParseSize(t *testing.T) {
	t.Parallel()

	sizeTests := []struct {
		size   string
		output interface{}
	}{
		{"full", SizeFull{}},
		{"pct:10", SizePercent{Percent: 10}},
		{"20,", SizeWidth{Width: 20}},
		{",30", SizeHeight{Height: 30}},
		{"40,50", SizeExact{Width: 40, Height: 50}},
		{"!60,70", SizeBestFit{Width: 60, Height: 70}},
	}

	for _, sizeTest := range sizeTests {
		o, err := parseSize(sizeTest.size)
		if err != nil {
			t.Errorf("Unexpected error parsing size: %s", sizeTest.size)
		}
		if o != sizeTest.output {
			t.Errorf("Incorrect output for %s\n", sizeTest.size)
		}
	}
}
