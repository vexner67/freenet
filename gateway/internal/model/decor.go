package model

// Decor is how the site is dressed for the New Year: Mode is what the
// administrators set — "auto" (by the calendar), "on" or "off" — and
// Festive whether the site wears the decoration now.
type Decor struct {
	Mode    string
	Festive bool
}
