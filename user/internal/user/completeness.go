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

// Completeness is how full the page is, in percent, as openvk counts it
// (without its rating), and which parts are missing ("interests",
// "email", "telegram", "status", "city"), for the owner's hints.
func (u *User) Completeness() (percent int, unfilled []string) {
	percent = 100
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
