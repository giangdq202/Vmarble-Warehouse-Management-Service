package domain

import "fmt"

// Dimension represents a rectangular measurement in millimeters.
type Dimension struct {
	LengthMM int `json:"length_mm"`
	WidthMM  int `json:"width_mm"`
}

func (d Dimension) AreaSqMM() int64 {
	return int64(d.LengthMM) * int64(d.WidthMM)
}

func (d Dimension) String() string {
	return fmt.Sprintf("%dx%dmm", d.LengthMM, d.WidthMM)
}

func (d Dimension) FitsInside(outer Dimension) bool {
	return d.LengthMM <= outer.LengthMM && d.WidthMM <= outer.WidthMM
}

func (d Dimension) Valid() bool {
	return d.LengthMM > 0 && d.WidthMM > 0
}
