package user

// The parts of a page openvk weighs in "Заполненность страницы", with
// what each adds; the order is that of its hints.
var completenessParts = []struct {
	name   string
	weight int
	filled func(u *User) bool
}{
	{"interests", 20, func(u *User) bool { return u.interests.Interests != "" }},
	{"email", 20, func(u *User) bool { return u.contacts.Email != "" }},
	{"telegram", 15, func(u *User) bool { return u.contacts.Telegram != "" }},
	{"status", 15, func(u *User) bool { return u.profile.Status != "" }},
	{"city", 20, func(u *User) bool { return u.contacts.City != "" }},
}

// Completeness is the rating of the page, in percent, as openvk counts
// it: how full the page is plus what vouchers added, so it may pass 100;
// and which parts are missing ("interests", "email", "telegram",
// "status", "city"), for the owner's hints.
func (u *User) Completeness() (percent int, unfilled []string) {
	percent = 100 + u.wallet.Rating
	for _, part := range completenessParts {
		if !part.filled(u) {
			percent -= part.weight
			unfilled = append(unfilled, part.name)
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
