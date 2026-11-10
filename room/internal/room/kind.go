package room

type Kind string

const (
	KindTalk   Kind = "talk"
	KindWatch  Kind = "watch"
	KindListen Kind = "listen"
)

func NewKind(s string) (Kind, error) {
	switch Kind(s) {
	case KindTalk, KindWatch, KindListen:
		return Kind(s), nil
	}
	return "", ErrInvalidKind
}
func (k Kind) String() string { return string(k) }
