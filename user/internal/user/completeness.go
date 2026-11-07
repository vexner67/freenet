package user

import "github.com/google/uuid"

// completenessField is a field of the page "Заполненность страницы" weighs
// and the percent it adds once filled in.
type completenessField struct {
	weight int
	filled func(u *User) bool
}

// completenessBlocks are the blocks of the page as "Редактировать
// страницу" groups them, with the photo and the status on their own: every
// field the site asks for, worth 100 percent in all. Each field adds its
// weight by itself, so the page fills up as it is filled in; the extra
// fields, named by the owner, add nothing.
//
// The migration add_score_to_users counts the same weights in SQL for the
// search order (users.score): changing one, change it there too.
var completenessBlocks = []struct {
	name   string
	fields []completenessField
}{
	{"photo", []completenessField{
		{20, func(u *User) bool { return u.avatarID != uuid.Nil }},
	}},
	{"status", []completenessField{
		{5, func(u *User) bool { return u.profile.Status != "" }},
	}},
	{"main", []completenessField{
		{3, func(u *User) bool { return u.profile.Surname != "" }},
		{3, func(u *User) bool { return u.profile.Gender != GenderNone }},
		{4, func(u *User) bool { return !u.profile.Birthday.Time().IsZero() }},
		{4, func(u *User) bool { return u.profile.Hometown != "" }},
		{3, func(u *User) bool { return u.profile.Relationship != RelationshipNone }},
		{4, func(u *User) bool { return u.profile.Worldview != WorldviewNone }},
		{4, func(u *User) bool { return u.profile.PoliticalViews != PoliticalViewsNone }},
	}},
	{"contacts", []completenessField{
		{4, func(u *User) bool { return u.contacts.Email != "" }},
		{4, func(u *User) bool { return u.contacts.Telegram != "" }},
		{4, func(u *User) bool { return u.contacts.Website != "" }},
		{4, func(u *User) bool { return u.contacts.City != "" }},
		{4, func(u *User) bool { return u.contacts.Address != "" }},
	}},
	{"interests", []completenessField{
		{4, func(u *User) bool { return u.interests.Interests != "" }},
		{4, func(u *User) bool { return u.interests.Music != "" }},
		{4, func(u *User) bool { return u.interests.Films != "" }},
		{4, func(u *User) bool { return u.interests.Shows != "" }},
		{4, func(u *User) bool { return u.interests.Books != "" }},
		{3, func(u *User) bool { return u.interests.Quotes != "" }},
		{3, func(u *User) bool { return u.interests.Games != "" }},
		{4, func(u *User) bool { return u.interests.About != "" }},
	}},
}

// Unfilled is a block of the page not filled in to the end: "photo",
// "status", "main", "contacts" or "interests", and the percent its empty
// fields would add.
type Unfilled struct {
	Part    string
	Percent int
}

// Completeness is the rating of the page, in percent: how full the page
// is (up to 100) plus the rating bought, at most MaxRating; and the blocks
// still missing something, in the order of the hints, for the owner.
func (u *User) Completeness() (percent int, unfilled []Unfilled) {
	percent, unfilled = u.fill()

	return min(percent+u.wallet.Rating, MaxRating), unfilled
}

// Fill is how full the page is, 0-100 percent, without the rating.
func (u *User) Fill() int {
	percent, _ := u.fill()

	return percent
}

// RatingMost is the most rating the page may buy up to: with how full the
// page is, it stays within MaxRating.
func (u *User) RatingMost() int {
	return MaxRating - u.Fill()
}

func (u *User) fill() (percent int, unfilled []Unfilled) {
	for _, block := range completenessBlocks {
		missing := 0
		for _, f := range block.fields {
			if f.filled(u) {
				percent += f.weight
			} else {
				missing += f.weight
			}
		}

		if missing > 0 {
			unfilled = append(unfilled, Unfilled{Part: block.name, Percent: missing})
		}
	}

	return percent, unfilled
}

// UpdateStatus replaces the status alone, as openvk's status edited on
// the page.
func (u *User) UpdateStatus(status Status) {
	u.profile.Status = status
}

// UpdateNickname replaces the address of the page alone ("Адрес страницы"
// of the settings), leaving the rest of the profile as it is. The page
// has to be filled first: nobody has an address without a name.
func (u *User) UpdateNickname(nickname Nickname) error {
	profile := u.profile
	profile.Nickname = nickname

	if err := profile.validate(); err != nil {
		return err
	}

	u.profile = profile

	return nil
}
