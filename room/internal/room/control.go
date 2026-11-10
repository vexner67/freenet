package room

type Control string

const (
	ControlOwner    Control = "owner"
	ControlEveryone Control = "everyone"
)

func NewControl(s string) (Control, error) {
	switch Control(s) {
	case ControlOwner, ControlEveryone:
		return Control(s), nil
	}
	return "", ErrInvalidControl
}
func (c Control) String() string { return string(c) }
