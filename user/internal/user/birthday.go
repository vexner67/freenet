package user

import (
	"errors"
	"time"
)

var ErrBirthdayInFuture = errors.New("birthday is in the future")

type Birthday time.Time

func NewBirthday(t time.Time) (Birthday, error) {
	if t.IsZero() {
		return Birthday{}, nil
	}

	now := time.Now()

	birthday := time.Date(
		t.Year(),
		t.Month(),
		t.Day(),
		0, 0, 0, 0,
		t.Location(),
	)

	today := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0, 0, 0, 0,
		now.Location(),
	)

	if birthday.After(today) {
		return Birthday{}, ErrBirthdayInFuture
	}

	return Birthday(birthday), nil
}

func (b Birthday) Time() time.Time {
	return time.Time(b)
}
