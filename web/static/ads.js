// The ads of the global feed, as Twitter slips promoted tweets between the
// others: a post of the site's own look with a gray head "Рекламная
// запись" instead of an author, the title, the text and the cover, all
// leading to the link in a new tab. openvk has no ads.

import { adDue } from './labels.js';
import { el, externalLink, textNode } from './layout.js';
import { tr } from './i18n.js';

function adLink(ad, className, children) {
  const link = externalLink(ad.url, children);
  link.className = className;
  link.rel = 'noopener noreferrer nofollow sponsored';

  return link;
}

// adNode is one ad of the feed.
export function adNode(ad) {
  return el('div', { className: 'post post-ad' },
    el('div', { className: 'post-main' },
      el('div', { className: 'post-author' },
        adLink(ad, 'post-name', [ad.title]),
        el('div', { className: 'post-date', textContent: tr('Рекламная запись') }),
      ),
      el('div', { className: 'post-content' },
        ad.text ? textNode(ad.text) : '',
        ad.photo ? adLink(ad, 'post-ad-cover', [el('img', { src: ad.photo.large, alt: '', loading: 'lazy' })]) : '',
      ),
      el('div', { className: 'post-menu' },
        el('span', {}, adLink(ad, 'post-ad-site', [ad.site])),
        el('span', {}, adLink(ad, 'post-ad-go', [tr('Перейти на сайт')])),
      ),
    ),
  );
}

// adStream puts the ads between the posts of a feed as they come: one
// after the third post and then after every eighth, in turn from a random
// one, so that the page shows another each time.
export function adStream(ads) {
  let shown = 0;
  let next = ads.length ? Math.floor(Math.random() * ads.length) : 0;

  // withAds are the nodes of the posts with the ads that fall among them.
  return (nodes) => nodes.flatMap((node) => {
    shown++;
    if (!ads.length || !adDue(shown)) return [node];

    return [node, adNode(ads[next++ % ads.length])];
  });
}
