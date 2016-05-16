package main

import "testing"

func TestParseSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
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

	for _, test := range tests {
		o, err := parseSize(test.size)
		if err != nil {
			t.Errorf("Unexpected error parsing size: %s", test.size)
		}
		if o != test.output {
			t.Errorf("Incorrect output for %s\n", test.size)
		}
	}
}

func TestParseRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		region string
		output interface{}
	}{
		{"full", RegionFull{}},
		{"10,20,30,40", RegionExact{X: 10, Y: 20, Width: 30, Height: 40}},
		{"pct:50,60.1,70.2,80.3",
			RegionPercent{
				X:      50,
				Y:      60.1,
				Width:  70.2,
				Height: 80.3,
			},
		},
	}

	for _, test := range tests {
		o, err := parseRegion(test.region)
		if err != nil {
			t.Errorf("Unexpected error parsing region: %s\n", test.region)
		}
		if o != test.output {
			t.Errorf("expected %v, got: %v\n", test.output, o)
		}
	}
}

func TestParseQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		quality string
		output  string
	}{
		{"color", "color"},
		{"gray", "gray"},
		{"bitonal", "bitonal"},
		{"default", "default"},
	}

	for _, test := range tests {
		o, err := parseQuality(test.quality)
		if err != nil {
			t.Errorf("Unexpected error parsing quality: %s\n", test.quality)
		}
		if *o != test.output {
			t.Errorf("expected %v, got: %v\n", test.output, o)
		}
	}

	_, err := parseQuality("invalidValue")
	if err.Error() != ErrInvalidQuality {
		t.Errorf("Expected ErrInvalidQuality")
	}
}

func TestParseFormta(t *testing.T) {
	t.Parallel()
}
