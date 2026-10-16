// Invitations, openvk's "Пригласить": a link "/?invite=<nickname>" brings
// a newcomer to the main page, which keeps who invited; once the newcomer
// has filled in the page, any page of the site accepts the invitation,
// so they follow the inviter, who hears of it.

import { api } from './api.js';
import { inviteNickname } from './labels.js';

const INVITE_KEY = 'freenet.invite';

// keepInvite keeps who invites by the address of this page; the nickname,
// or '' for nobody.
export function keepInvite() {
  const nickname = inviteNickname(location.search);

  if (nickname) {
    try {
      localStorage.setItem(INVITE_KEY, nickname);
    } catch {
      // Not kept: the invitation is lost, the sign-up is not.
    }
  }

  return nickname;
}

// acceptInvite accepts the invitation kept in this browser for me, whose
// page is filled in. It is tried once: an old page, a second invitation
// or one's own link are refused by the server, and nothing is to be done
// about it.
export async function acceptInvite(me) {
  let nickname;
  try {
    nickname = localStorage.getItem(INVITE_KEY);
    if (!nickname || !me.profileCompleted) return;
    localStorage.removeItem(INVITE_KEY);
  } catch {
    return;
  }

  if (nickname.toLowerCase() === me.nickname.toLowerCase()) return;

  await api.acceptInvite(nickname).catch(() => {});
}
