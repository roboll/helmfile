package diff

// SliceOrdering determines whether the ordering of items in a slice results in a change
func SliceOrdering(enabled bool) func(d *Differ) error {
	return func(d *Differ) error {
		d.SliceOrdering = enabled
		return nil
	}
}

// DisableStructValues disables populating a seperate change for each item in a struct,
// where the struct is being compared to a nil value
func DisableStructValues() func(d *Differ) error {
	return func(d *Differ) error {
		d.DisableStructValues = true
		return nil
	}
}
