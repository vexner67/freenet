// Which nicknames are someone's. A text says "@word" of anything, and
// only the server knows whether there is such a page: the words of a page
// are asked about together, each once.

// A request names so many nicknames at most: its address stays short.
const BATCH = 50;

// nicknameLookup returns known(nickname), a promise of whether anyone has
// the nickname, whatever the case. fetchKnown(nicknames) asks the server
// and resolves with those that are someone's; later runs the request once
// the page has named all it shows now. A failed request rejects those who
// waited for it and is tried again when they ask again.
export function nicknameLookup(fetchKnown, later = (fn) => setTimeout(fn, 0)) {
  const answers = new Map();
  let waiting = [];

  const ask = async (batch) => {
    try {
      const found = new Set((await fetchKnown(batch.map((w) => w.key))).map((n) => n.toLowerCase()));
      for (const w of batch) w.resolve(found.has(w.key));
    } catch (err) {
      for (const w of batch) {
        answers.delete(w.key);
        w.reject(err);
      }
    }
  };

  const flush = () => {
    const asked = waiting;
    waiting = [];
    for (let i = 0; i < asked.length; i += BATCH) ask(asked.slice(i, i + BATCH));
  };

  return (nickname) => {
    const key = nickname.toLowerCase();
    if (answers.has(key)) return answers.get(key);

    const answer = new Promise((resolve, reject) => {
      if (waiting.length === 0) later(flush);
      waiting.push({ key, resolve, reject });
    });
    answers.set(key, answer);

    return answer;
  };
}
