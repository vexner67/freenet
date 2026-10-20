// The ads of the news, as Twitter slips promoted tweets between the
// others: a post of the site's own look with a gray head "Рекламная
// запись" instead of an author, the title, the text and the cover, all
// leading to the link in a new tab. Everyone gets them, whatever the
// settings of the feed say. Each ad counts once that it was seen (half
// of it on the screen) and each time that its link was followed, for the
// stats of the administrators. openvk has no ads.

import { api } from './api.js';
import { adDue } from './labels.js';
import { el, externalLink, textNode } from './layout.js';
import { tr } from './i18n.js';

// Counting fails quietly: the stats are not worth an error on the feed.
const quietly = (counted) => counted.catch(() => {});

function adLink(ad, className, children) {
  const link = externalLink(ad.url, children);
  link.className = className;
  link.rel = 'noopener noreferrer nofollow sponsored';

  // A click and a middle click both open the link.
  const clicked = (event) => {
    if (event.type === 'click' || event.button === 1) quietly(api.clickAd(ad.id));
  };
  link.addEventListener('click', clicked);
  link.addEventListener('auxclick', clicked);

  return link;
}

// seen counts an ad once it is half on the screen, once per its node.
const seen = typeof IntersectionObserver === 'undefined' ? null : new IntersectionObserver((entries) => {
  for (const entry of entries) {
    if (!entry.isIntersecting) continue;

    seen.unobserve(entry.target);
    quietly(api.viewAd(entry.target.dataset.ad));
  }
}, { threshold: 0.5 });

// adNode is one ad of the feed.
export function adNode(ad) {
  const node = el('div', { className: 'post post-ad' },
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
  node.dataset.ad = ad.id;
  seen?.observe(node);

  return node;
}

// adStream puts the ads between the posts of a feed as they come: one
// after the third post and then after every eighth, in turn from a random
// one, so that the page shows another each time. A feed that ends before
// its third post has its ad after the last one.
export function adStream(ads) {
  let shown = 0;
  let placed = 0;
  let next = ads.length ? Math.floor(Math.random() * ads.length) : 0;

  const ad = () => {
    placed++;
    return adNode(ads[next++ % ads.length]);
  };

  // withAds are the nodes of the posts with the ads that fall among them;
  // last says that no more posts follow.
  return (nodes, last = false) => {
    const all = nodes.flatMap((node) => {
      shown++;
      return ads.length && adDue(shown) ? [node, ad()] : [node];
    });

    if (last && ads.length && shown > 0 && placed === 0) all.push(ad());

    return all;
  };
}
