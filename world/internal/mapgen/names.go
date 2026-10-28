package mapgen

import (
	"math/rand/v2"
	"strings"
	"unicode"
	"unicode/utf8"
)

// culture is a way of naming: its syllables and endings.
type culture struct {
	onsets  []string
	vowels  []string
	codas   []string
	endings []string
}

// The names of the world are made up, in six styles of sound.
var cultureSounds = []culture{
	{ // northern
		onsets:  []string{"б", "в", "г", "д", "к", "л", "м", "н", "р", "с", "т", "ст", "бр", "гр", "тр", "св"},
		vowels:  []string{"а", "е", "и", "о", "у", "ё"},
		codas:   []string{"", "", "р", "н", "л", "рг", "нд", "льд"},
		endings: []string{"гард", "хейм", "ланд", "вик", "борг", "ния", ""},
	},
	{ // southern
		onsets:  []string{"с", "м", "л", "р", "т", "в", "к", "ф", "пр", "кл"},
		vowels:  []string{"а", "а", "е", "и", "о", "и"},
		codas:   []string{"", "", "", "н", "р", "с", "л"},
		endings: []string{"ия", "ана", "ено", "ица", "ора", "этто", ""},
	},
	{ // steppe
		onsets:  []string{"к", "т", "б", "с", "ж", "ч", "ш", "ар", "ул", "ер", "ак"},
		vowels:  []string{"а", "ы", "у", "е", "о", "а"},
		codas:   []string{"", "н", "р", "к", "л", "й", "т"},
		endings: []string{"стан", "кент", "абад", "ар", "кан", "ай", ""},
	},
	{ // eastern
		onsets:  []string{"к", "т", "х", "с", "м", "н", "р", "ш", "ц", "л"},
		vowels:  []string{"а", "и", "о", "у", "э", "я"},
		codas:   []string{"", "", "", "н", "ш"},
		endings: []string{"ан", "ин", "ото", "аки", "сэ", ""},
	},
	{ // western
		onsets:  []string{"в", "м", "л", "р", "д", "б", "ф", "гл", "др", "кр", "пл"},
		vowels:  []string{"а", "е", "и", "о", "у"},
		codas:   []string{"", "н", "р", "т", "нт", "рн", "ль"},
		endings: []string{"шир", "мут", "форд", "тон", "вель", "мон", ""},
	},
	{ // highland
		onsets:  []string{"д", "г", "к", "з", "т", "вл", "др", "зв", "гв", "мц"},
		vowels:  []string{"а", "е", "и", "о", "у"},
		codas:   []string{"", "р", "н", "л", "в", "д"},
		endings: []string{"ети", "ави", "уни", "ара", "ешт", ""},
	},
}

// namer gives unique names, each in the sounds of a culture.
type namer struct {
	r    *rand.Rand
	used map[string]bool
}

func (n *namer) name(c int, longer bool) string {
	style := cultureSounds[c%len(cultureSounds)]
	for attempt := 0; ; attempt++ {
		syllables := 1 + n.r.IntN(2)
		if longer || attempt > 20 {
			syllables++
		}
		var b strings.Builder
		for range syllables {
			b.WriteString(pick(n.r, style.onsets))
			b.WriteString(pick(n.r, style.vowels))
			b.WriteString(pick(n.r, style.codas))
		}
		b.WriteString(pick(n.r, style.endings))

		s := b.String()
		if l := utf8.RuneCountInString(s); l < 4 || l > 14 {
			continue
		}
		s = capital(s)
		if !n.used[s] {
			n.used[s] = true
			return s
		}
	}
}

func pick(r *rand.Rand, from []string) string {
	return from[r.IntN(len(from))]
}

func capital(s string) string {
	first, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(first)) + s[size:]
}

// name names the provinces, the seas and the great rivers.
func name(p Params, g grid, m *Map) {
	n := &namer{r: stream(p.Seed, streamNames), used: map[string]bool{}}
	for k := range m.Provinces {
		pr := &m.Provinces[k]
		if pr.Sea {
			pr.Name = "Море " + n.name(pr.Culture, false)
			continue
		}
		pr.Name = n.name(pr.Culture, false)
	}

	for k := range m.Rivers {
		rv := &m.Rivers[k]
		mouth := rv.Points[len(rv.Points)-1]
		i := int(mouth[1])*g.w + int(mouth[0])
		rv.Name = n.name(m.Provinces[m.Province[i]].Culture, false)
	}
}
