// The articles of a person at /articles?user=<nickname> (one's own without
// it), newest first, as the cards a post shows, with "Написать статью" on
// one's own.

import { api, currentUser, started } from './api.js';
import { articleCard, articlesCount } from './articles.js';
import { genitiveName } from './labels.js';
import { $, renderChrome, showError } from './layout.js';
import { tr } from './i18n.js';

const asked = new URLSearchParams(location.search).get('user') ?? '';

let cursor = '';

function show(page, mine) {
  $('summary').textContent = mine ? tr('У Вас {0}', articlesCount(page.total)) : articlesCount(page.total);
  $('summary').hidden = page.total === 0;
  $('empty').hidden = page.total !== 0;
  $('articles').append(...page.articles.map(articleCard));
  cursor = page.nextCursor;
  $('more').hidden = !cursor;
}

// The articles are asked for with who is signed in, in one round trip.
const wanted = asked ? started(api.articles(asked)) : null;
const me = await currentUser().catch(() => null);
renderChrome(me);

const nickname = asked || me?.nickname;
const mine = Boolean(me) && nickname === me.nickname;

if (!nickname) {
  location.replace('/');
} else {
  $('write').hidden = !mine || !me.profileCompleted;
  $('all-tab').href = '/articles?user=' + encodeURIComponent(nickname);

  try {
    const page = await (wanted ?? api.articles(nickname));
    if (!mine && page.articles[0]) {
      const author = page.articles[0].author;
      $('title').textContent = tr('Статьи {0}', genitiveName(author));
    }
    show(page, mine);
  } catch (e) {
    showError($('error'), e);
  }

  $('more').addEventListener('click', async () => {
    $('more').hidden = true;
    try {
      show(await api.articles(nickname, cursor), mine);
    } catch (e) {
      showError($('error'), e);
    }
  });
}
