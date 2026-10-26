// openvk's "Двухфакторная аутентификация" of "Безопасность": an
// authenticator app (TOTP) gives the code that signing in asks for after
// the email code. "Подключить" shows the QR code and the key and takes the
// first code; the recovery codes are shown once, when it is turned on and
// when new ones are made; turning it off takes a code too.

import { api } from './api.js';
import { qrCells, recoveryLeftText } from './labels.js';
import { $, el, hideError, showError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// A QR code needs a white margin of four modules to be read.
const quietZone = 4;
const modulePx = 4;

// qrCanvas paints the QR code dark on white whatever the theme: apps read
// it only so.
function qrCanvas(rows) {
  const side = (rows.length + quietZone * 2) * modulePx;
  const canvas = el('canvas', { className: 'qr-code', width: side, height: side });
  const ctx = canvas.getContext('2d');

  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, side, side);
  ctx.fillStyle = '#000';
  for (const [x, y] of qrCells(rows)) {
    ctx.fillRect((x + quietZone) * modulePx, (y + quietZone) * modulePx, modulePx, modulePx);
  }

  canvas.setAttribute('role', 'img');
  canvas.setAttribute('aria-label', tr('QR-код для приложения'));

  return canvas;
}

function codeInput() {
  return el('input', { type: 'text', maxLength: 11, autocomplete: 'one-time-code', spellcheck: false, required: true, ariaLabel: tr('Код') });
}

// askForm is a box with a form whose button sends the code to act and
// shows what fails in the box.
function askForm(title, intro, label, act) {
  const modal = openModal(title, { tag: 'form' });
  const fail = el('div', { className: 'error', hidden: true });
  const code = codeInput();
  const send = button(label, null, { type: 'submit' });

  modal.body.append(fail, ...intro,
    el('table', { className: 'form two-factor-form' }, el('tr', {},
      el('th', {}, el('label', { textContent: tr('Код:') })),
      el('td', { className: 'control' }, code))));
  modal.actions.append(send, button(tr('Отмена'), modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(fail);
    send.disabled = true;

    try {
      await act(code.value.trim(), modal);
    } catch (e) {
      showError(fail, e);
      send.disabled = false;
      code.select();
    }
  });
  code.focus();

  return modal;
}

// showRecovery shows the recovery codes this once, with a file to keep.
function showRecovery(codes) {
  const modal = openModal(tr('Резервные коды для подтверждения входа'));
  const file = new Blob([`Freenet\n\n${codes.join('\n')}\n`], { type: 'text/plain' });
  const url = URL.createObjectURL(file);
  const save = el('a', { className: 'button', href: url, download: 'freenet-recovery-codes.txt', textContent: tr('Сохранить в файл') });

  modal.body.append(
    el('p', { textContent: tr('Резервные коды позволяют подтверждать вход, когда у вас нет доступа к телефону, например, в путешествии.') }),
    el('p', {}, tr('Каждым кодом можно воспользоваться только один раз. '), el('b', { textContent: tr('Запишите или сохраните их сейчас') }),
      tr(': больше сайт их не покажет.')),
    el('ol', { className: 'recovery-codes' }, ...codes.map((c) => el('li', { textContent: c }))),
    el('p', { className: 'hint', textContent: tr('Вы можете получить новые коды, если они заканчиваются. Действительны только последние созданные резервные коды.') }));
  modal.actions.append(save, button(tr('Готово'), () => {
    URL.revokeObjectURL(url);
    modal.close();
  }));
}

// openSetup is openvk's page of "Подключить", in a box.
async function openSetup(done) {
  const setup = await api.setUpTwoFactor();

  askForm(tr('Двухфакторная аутентификация'), [
    el('p', {}, tr('Двухфакторную аутентификацию через TOTP можно использовать даже без интернета. Для этого вам понадобится приложение для генерации кодов. Например, '),
      el('b', { textContent: 'Google Authenticator' }), ', ', el('b', { textContent: tr('Яндекс Ключ') }), tr(' или свободное '),
      el('b', { textContent: 'Aegis' }), tr('. Убедитесь, что на телефоне точно установлены дата и время.')),
    el('p', { textContent: tr('Используя приложение для двухфакторной аутентификации, отсканируйте приведённый ниже QR-код:') }),
    el('div', { className: 'qr-box' }, qrCanvas(setup.qr)),
    el('p', { textContent: tr('или вручную введите секретный ключ:') }),
    el('p', { className: 'safety-code secret-key', textContent: setup.secret }),
    el('p', { textContent: tr('Теперь введите код, который вам предоставило приложение, чтобы мы могли подтвердить, что вы действительно вы.') }),
  ], tr('Включить'), async (code, modal) => {
    const { recoveryCodes } = await api.enableTwoFactor(code);
    modal.close();
    done(tr('Двухфакторная аутентификация включена'), tr('Вашу страницу стало труднее взломать.'));
    showRecovery(recoveryCodes);
  });
}

// showTwoFactor draws the block of the settings in its state; say tells
// what was done above the settings.
export async function showTwoFactor(error, say) {
  const box = $('two-factor');
  const redraw = (title, text) => {
    if (title) say(title, ...(text ? [text] : []));
    showTwoFactor(error, say);
  };

  let state;
  try {
    state = await api.twoFactor();
  } catch (e) {
    showError(error, e);
    return;
  }

  if (!state.enabled) {
    box.replaceChildren(
      el('div', { className: 'accent-box', textContent: tr('Обеспечивает надёжную защиту от взлома: для входа на страницу необходимо ввести код, полученный в приложении 2FA.') }),
      el('div', { className: 'center' }, button(tr('Подключить'), () => openSetup(redraw).catch((e) => showError(error, e)))));
    return;
  }

  box.replaceChildren(
    el('div', { className: 'accent-box' },
      el('p', { textContent: tr('Двухфакторная аутентификация включена. Ваша страница защищена.') }),
      el('p', { className: 'hint', textContent: recoveryLeftText(state.recoveryLeft) })),
    el('div', { className: 'key-actions' },
      button(tr('Новые резервные коды'), () => askForm(tr('Новые резервные коды'),
        [el('p', { textContent: tr('Введите код из приложения. Прежние резервные коды перестанут действовать.') })],
        tr('Получить'), async (code, modal) => {
          const { recoveryCodes } = await api.renewRecoveryCodes(code);
          modal.close();
          redraw();
          showRecovery(recoveryCodes);
        })),
      button(tr('Отключить'), () => askForm(tr('Отключить 2FA'),
        [el('p', { textContent: tr('Введите код из приложения или резервный код. Без второго шага для входа будет достаточно кода из письма.') })],
        tr('Отключить'), async (code, modal) => {
          await api.disableTwoFactor(code);
          modal.close();
          redraw(tr('Двухфакторная аутентификация отключена'));
        }), { secondary: true })));
}
