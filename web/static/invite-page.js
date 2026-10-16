// "Пригласить" of openvk: the link that invites in the name of the page.

import { currentUser } from './api.js';
import { inviteURL } from './labels.js';
import { $, renderChrome } from './layout.js';

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);

  if (me.profileCompleted) {
    const link = $('invite-link');
    link.value = inviteURL(location.origin, me.nickname);
    link.addEventListener('focus', () => link.select());
    $('copy').addEventListener('click', () => copy(link));
  } else {
    $('invite').hidden = true;
    $('no-nickname').hidden = false;
  }
}

// copy puts the link on the clipboard, or leaves it selected to be
// copied by hand where the browser does not let a page write there.
async function copy(link) {
  link.select();

  try {
    await navigator.clipboard.writeText(link.value);
  } catch {
    return;
  }

  $('copied').hidden = false;
}
