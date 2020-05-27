package plugins

import "testing"

func TestValsInstance(t *testing.T) {
	i, err := ValsInstance()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	i2, _ := ValsInstance()

	if i != i2 {
		t.Error("Instances should be equal")
	}
}
