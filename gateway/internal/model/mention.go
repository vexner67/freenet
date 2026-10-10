package model

import (
	"regexp"
	"strings"
)

// MaxMentions is how many people one text tells that it names them.
const MaxMentions = 10

// maxNicknameLen is the longest nickname the user service gives.
const maxNicknameLen = 32

// mentionPattern finds openvk's two ways to name a person in a text:
// "[nickname|Имя]", which "Ответить" writes, and "@nickname" at the start
// or after a space. The site draws both as links to the page (labels.js
// has the same pattern).
var mentionPattern = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9_]*)\|[^\[\]|\n]{1,100}\]|(?:^|\s)@([A-Za-z][A-Za-z0-9_]*)`)

// MentionedNicknames are the nicknames text names, each once whatever the
// case, in the order named, the first MaxMentions of them. Whether anyone
// has such a nickname the user service knows.
func MentionedNicknames(text string) []string {
	var (
		found []string
		seen  = make(map[string]bool)
	)

	for _, m := range mentionPattern.FindAllStringSubmatch(text, -1) {
		nickname := m[1]
		if nickname == "" {
			// A nickname does not end with "_": "@ann_" names ann.
			nickname = strings.TrimRight(m[2], "_")
		}

		key := strings.ToLower(nickname)
		if len(nickname) > maxNicknameLen || strings.HasSuffix(nickname, "_") || seen[key] {
			continue
		}

		seen[key] = true
		found = append(found, nickname)

		if len(found) == MaxMentions {
			break
		}
	}

	return found
}
