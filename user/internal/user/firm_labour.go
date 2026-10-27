package user

import (
	"math"
	"slices"
	"time"
)

// The labour market: the enterprises hire the people of the city. Who is
// out of work goes to the one paying the most; who works for less than
// the city pays on average leaves for one paying more with a vacancy. The
// city grows towards the jobs it has — people come when there are more,
// and go, of those out of work, when fewer — but never fills them all, so
// that the enterprises vie for workers with their wages.

const (
	// labourShare is the share of the jobs of the city its people grow
	// to fill.
	labourShare = 0.95
	// labourDrift is the share of the gap between the people and the jobs
	// that comes or goes in an hour.
	labourDrift = 0.1
	// maxQuit is the most of its workers an enterprise loses in an hour.
	maxQuit = 0.5
)

// Labour is the labour market of the city: its people, those working,
// the workers the enterprises seek in all and the wage they pay on
// average, in hundredths an hour.
type Labour struct {
	People   int
	Employed int
	Jobs     int
	Wage     int64
}

// Unemployed are the people out of work.
func (l Labour) Unemployed() int {
	return max(l.People-l.Employed, 0)
}

// Labour is the labour market now.
func (c *City) Labour(rules FirmRules) Labour {
	l := Labour{People: c.people}
	var paid int64
	for _, f := range c.firms {
		l.Employed += f.workers
		l.Jobs += f.wanted
		paid += int64(f.workers) * f.wage
	}

	l.Wage = rules.Wage
	if l.Employed > 0 {
		l.Wage = paid / int64(l.Employed)
	}
	l.People = max(l.People, l.Employed)

	return l
}

// byWage are the enterprises paying the most first; of those paying the
// same, each hour another one first, as in the hour of work.
func (c *City) byWage(at time.Time) []*Firm {
	n := len(c.firms)
	if n == 0 {
		return nil
	}

	first := int(at.Unix()/3600) % n
	firms := append(append([]*Firm(nil), c.firms[first:]...), c.firms[:first]...)
	slices.SortStableFunc(firms, func(a, b *Firm) int {
		switch {
		case a.wage > b.wage:
			return -1
		case a.wage < b.wage:
			return 1
		default:
			return 0
		}
	})

	return firms
}

// labour is the hour at on the labour market: workers leave for better
// pay, the city grows or shrinks towards its jobs, and those out of work
// are hired.
func (c *City) labour(rules FirmRules, at time.Time) {
	c.poach(rules, at)
	c.migrate()
	c.hire(at)
}

// poach moves the workers of the enterprises paying below the average to
// those paying more with vacancies: the further below, the more leave.
func (c *City) poach(rules FirmRules, at time.Time) {
	wage := c.Labour(rules).Wage
	firms := c.byWage(at)

	for i := len(firms) - 1; i >= 0; i-- {
		from := firms[i]
		if from.workers == 0 || from.wage >= wage {
			continue
		}

		share := min(float64(wage-from.wage)/float64(wage), maxQuit)
		leave := int(math.Round(float64(from.workers) * share))
		for _, to := range firms {
			if leave == 0 || to.wage <= from.wage {
				break
			}

			moved := min(leave, to.wanted-to.workers)
			if moved <= 0 {
				continue
			}

			from.workers -= moved
			to.workers += moved
			leave -= moved
			from.hour(at).Left += int64(moved)
			to.hour(at).Hired += int64(moved)
		}
	}
}

// migrate brings people to the city or takes those out of work away, a
// share of the gap to the jobs it fills.
func (c *City) migrate() {
	employed, jobs := 0, 0
	for _, f := range c.firms {
		employed += f.workers
		jobs += f.wanted
	}

	target := int(math.Ceil(float64(jobs) * labourShare))
	gap := float64(target - c.people)
	step := int(math.Ceil(math.Abs(gap) * labourDrift))
	if gap < 0 {
		step = -step
	}

	c.people = max(c.people+step, employed)
}

// hire gives the people out of work to the enterprises with vacancies,
// those paying the most first.
func (c *City) hire(at time.Time) {
	free := c.people
	for _, f := range c.firms {
		free -= f.workers
	}

	for _, f := range c.byWage(at) {
		if free <= 0 {
			return
		}

		n := min(free, f.wanted-f.workers)
		if n <= 0 {
			continue
		}

		f.workers += n
		free -= n
		f.hour(at).Hired += int64(n)
	}
}
