package user

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// Trait is what a worker is like, past the skill.
type Trait string

const (
	TraitNone Trait = ""
	// TraitFast serves a fifth more.
	TraitFast Trait = "fast"
	// TraitStudious learns twice as fast.
	TraitStudious Trait = "studious"
	// TraitLazy serves a fifth less and asks a fifth less.
	TraitLazy Trait = "lazy"
	// TraitCalm does not mind too much work.
	TraitCalm Trait = "calm"
)

// The staff of a business: a worker learns the next skill in
// skillHours×skill hours of work, and his mood drifts to calmMood.
const (
	MaxSkill        = 5
	CandidatesShown = 5
	candidateWindow = 6 * time.Hour
	skillHours      = 96
	calmMood        = 70
	gloomyMood      = 40
	quittingMood    = 20
	bonusHours      = 8
)

var (
	ErrCandidateGone  = errors.New("this candidate is gone, look at the new ones")
	ErrNoSeats        = errors.New("there are no free seats: upgrade the business")
	ErrWorkerNotFound = errors.New("there is no such worker")
)

// Worker is a worker of a business: his profession and skill (1–5), the
// hours he worked toward the next skill, his wage an hour in hundredths
// of a vote, his trait and his mood (0–100: below 40 he works worse,
// below 20 he quits).
type Worker struct {
	ID         int
	Name       string
	Female     bool
	Profession Profession
	Skill      int
	XP         float64
	Wage       int64
	Trait      Trait
	Mood       float64
}

// skillRate is how a skill works: 1 at skill 2, from 0.75 to 1.75.
func skillRate(skill int) float64 {
	return 0.5 + 0.25*float64(skill)
}

// rate is how much of his profession w does: the skill, the trait and the
// mood.
func (w Worker) rate() float64 {
	r := skillRate(w.Skill)
	switch w.Trait {
	case TraitFast:
		r *= 1.2
	case TraitLazy:
		r *= 0.8
	}
	if w.Mood < gloomyMood {
		r *= 0.8
	}

	return r
}

// Rate is how much of his profession w does: 1 at skill 2 in a calm
// mood.
func (w Worker) Rate() float64 {
	return w.rate()
}

// BonusCost is the votes a bonus of w costs: eight hours of the wage,
// rounded up.
func (w Worker) BonusCost() int64 {
	return (w.Wage*bonusHours + 99) / 100
}

// NextSkillHours is how many hours of work w needs for the next skill; 0
// at the top one.
func (w Worker) NextSkillHours() float64 {
	if w.Skill >= MaxSkill {
		return 0
	}

	return float64(skillHours * w.Skill)
}

// wageOf is the wage a worker of skill and trait asks.
func wageOf(spec ProfessionSpec, skill int, trait Trait, r BusinessRules) int64 {
	wage := float64(spec.Wage) * (0.6 + 0.2*float64(skill)) * float64(r.Wages()) / 100
	if trait == TraitLazy {
		wage *= 0.8
	}

	return max(int64(math.Round(wage)), 1)
}

// skillOf is the skill of the most skilled worker of a profession, 0
// without one.
func (b Business) skillOf(p Profession) int {
	skill := 0
	for _, w := range b.workers {
		if w.Profession == p {
			skill = max(skill, w.Skill)
		}
	}

	return skill
}

func (b Business) worker(id int) (int, error) {
	for i, w := range b.workers {
		if w.ID == id {
			return i, nil
		}
	}

	return 0, ErrWorkerNotFound
}

// Wages is what the workers of b get an hour, in hundredths of a vote.
func (b Business) Wages() int64 {
	var wages int64
	for _, w := range b.workers {
		wages += w.Wage
	}

	return wages
}

func candidateWindowAt(now time.Time) int64 {
	return now.Unix() / int64(candidateWindow/time.Second)
}

// CandidatesRenewAt is when the candidates of now change.
func CandidatesRenewAt(now time.Time) time.Time {
	return time.Unix((candidateWindowAt(now)+1)*int64(candidateWindow/time.Second), 0).UTC()
}

// Candidate is a worker b may hire, known by ID until the candidates
// change.
type Candidate struct {
	ID     int64
	Worker Worker
}

// Candidates are who b may hire now: five every six hours, at least two
// of the professions of its offers, the hired ones gone. They are the
// same for anyone who looks in the same six hours.
func (b Business) Candidates(r BusinessRules, now time.Time) []Candidate {
	spec, err := BusinessCatalog(b.kind)
	if err != nil {
		return nil
	}

	window := candidateWindowAt(now)
	rng := rand.New(rand.NewPCG(b.seed, uint64(window)*0x9E3779B97F4A7C15+1)) //nolint:gosec // the game, not secrets
	serving := spec.Serving()

	var candidates []Candidate
	for slot := range CandidatesShown {
		prof := spec.Professions[rng.IntN(len(spec.Professions))].Profession
		if slot < 2 {
			prof = serving[rng.IntN(len(serving))]
		}

		w := randomWorker(rng, spec, prof, r)
		if window == b.window && slices.Contains(b.hired, slot) {
			continue
		}

		candidates = append(candidates, Candidate{ID: window*CandidatesShown + int64(slot), Worker: w})
	}

	return candidates
}

func randomWorker(rng *rand.Rand, spec KindSpec, prof Profession, r BusinessRules) Worker {
	skill := 1
	for roll, edges := rng.IntN(100), []int{30, 60, 82, 95}; skill <= len(edges) && roll >= edges[skill-1]; {
		skill++
	}

	traits := []Trait{TraitNone, TraitNone, TraitFast, TraitStudious, TraitLazy, TraitCalm}
	trait := traits[rng.IntN(len(traits))]
	female := rng.IntN(2) == 0
	name := randomName(rng, female)
	ps, _ := spec.Profession(prof)

	return Worker{
		Name: name, Female: female, Profession: prof, Skill: skill,
		Wage: wageOf(ps, skill, trait, r), Trait: trait, Mood: calmMood,
	}
}

var (
	maleNames = []string{
		"Александр", "Алексей", "Андрей", "Антон", "Артём", "Борис", "Вадим", "Василий", "Виктор", "Владимир",
		"Геннадий", "Георгий", "Григорий", "Денис", "Дмитрий", "Евгений", "Егор", "Иван", "Игорь", "Илья",
		"Кирилл", "Константин", "Леонид", "Максим", "Михаил", "Никита", "Николай", "Олег", "Павел", "Пётр",
		"Роман", "Сергей", "Степан", "Тимур", "Фёдор", "Юрий",
	}
	femaleNames = []string{
		"Александра", "Алина", "Алла", "Анастасия", "Анна", "Валентина", "Валерия", "Вера", "Галина", "Дарья",
		"Евгения", "Екатерина", "Елена", "Жанна", "Зоя", "Инна", "Ирина", "Карина", "Ксения", "Лариса",
		"Людмила", "Марина", "Мария", "Надежда", "Наталья", "Нина", "Оксана", "Ольга", "Полина", "Светлана",
		"Софья", "Татьяна", "Ульяна", "Юлия", "Яна",
	}
	surnames = []string{
		"Иванов", "Петров", "Сидоров", "Смирнов", "Кузнецов", "Попов", "Соколов", "Лебедев", "Козлов", "Новиков",
		"Морозов", "Волков", "Соловьёв", "Васильев", "Зайцев", "Павлов", "Семёнов", "Голубев", "Виноградов", "Богданов",
		"Воробьёв", "Фёдоров", "Михайлов", "Беляев", "Тарасов", "Белов", "Комаров", "Орлов", "Киселёв", "Макаров",
		"Андреев", "Ковалёв", "Ильин", "Гусев", "Титов", "Кузьмин", "Кудрявцев", "Баранов", "Куликов", "Алексеев",
		"Степанов", "Яковлев", "Сорокин", "Сергеев", "Романов", "Захаров", "Борисов", "Королёв", "Герасимов", "Пономарёв",
	}
)

func randomName(rng *rand.Rand, female bool) string {
	surname := surnames[rng.IntN(len(surnames))]
	if female {
		return femaleNames[rng.IntN(len(femaleNames))] + " " + surname + "а"
	}

	return maleNames[rng.IntN(len(maleNames))] + " " + surname
}

// Hired is b with the candidate of now on its staff.
func (b Business) Hired(p BusinessPlan, r BusinessRules, candidate int64, now time.Time) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}
	if len(b.workers) >= b.Seats(p) {
		return Business{}, ErrNoSeats
	}

	var hired *Candidate
	for _, c := range b.Candidates(r, now) {
		if c.ID == candidate {
			hired = &c

			break
		}
	}
	if hired == nil {
		return Business{}, ErrCandidateGone
	}

	b = b.clone()
	if window := candidateWindowAt(now); b.window != window {
		b.window, b.hired = window, nil
	}
	b.hired = append(b.hired, int(candidate%CandidatesShown))

	w := hired.Worker
	w.ID = b.nextID
	b.nextID++
	b.workers = append(b.workers, w)
	b.note(Note{At: now.UTC(), Kind: NoteHired, Subject: string(w.Profession), Name: w.Name, Female: w.Female})

	return b, nil
}

// Fired is b without a worker.
func (b Business) Fired(id int, now time.Time) (Business, error) {
	i, err := b.worker(id)
	if err != nil {
		return Business{}, err
	}

	b = b.clone()
	w := b.workers[i]
	b.workers = slices.Delete(b.workers, i, i+1)
	b.note(Note{At: now.UTC(), Kind: NoteFired, Subject: string(w.Profession), Name: w.Name, Female: w.Female})

	return b, nil
}

// Bonused is b with a worker paid eight hours of the wage, whole votes
// rounded up, and that cost: his mood rises a quarter.
func (b Business) Bonused(id int) (Business, int64, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, 0, err
	}

	i, err := b.worker(id)
	if err != nil {
		return Business{}, 0, err
	}

	b = b.clone()
	cost := b.workers[i].BonusCost()
	b.workers[i].Mood = min(b.workers[i].Mood+25, 100)

	return b, cost, nil
}

// Raised is b with a wage a tenth higher, rounded up: the mood rises.
func (b Business) Raised(id int) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}

	i, err := b.worker(id)
	if err != nil {
		return Business{}, err
	}

	b = b.clone()
	b.workers[i].Wage = min(b.workers[i].Wage+(b.workers[i].Wage+9)/10, MaxBusinessPrice)
	b.workers[i].Mood = min(b.workers[i].Mood+15, 100)

	return b, nil
}

// FirstName is the first word of a name.
func FirstName(name string) string {
	first, _, _ := strings.Cut(name, " ")

	return first
}
