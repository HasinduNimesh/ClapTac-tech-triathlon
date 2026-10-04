package handler

import "testing"

func TestCSVSafeNeutralisesFormulaCells(t *testing.T) {
	for in, want := range map[string]string{
		"=1+1": "'=1+1", "+cmd": "'+cmd", "-2": "'-2", "@SUM(A1)": "'@SUM(A1)", "\tx": "'\tx", "\rx": "'\rx",
		"plain": "plain", "": "", "a=b": "a=b",
	} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q)=%q want %q", in, got, want)
		}
	}
}
