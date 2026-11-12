import './static-page.js';
import { api } from './api.js';
import { $, avatarURL, el } from './layout.js';

const owner = await api.profile('arm').catch(() => null);
if (owner && !owner.deactivated && !owner.ban) {
  $('privacy-owner-name').prepend(el('img', { src: avatarURL(owner, 'small'), alt: '', width: 40, height: 40 }));
}
