// Videos of YouTube in messages, as openvk's: "Видеозапись" of
// "Прикрепить" opens its "Загрузить видео" (showFastVideoUpload in
// Web/static/js/al_wall.js) at "Добавить с YouTube", and a message shows a
// video as its msg-attach-j-video. YouTube is contacted only after the
// reader chooses to load the player. The site keeps no videos of its own: a video
// is its link, a line of the message's words (splitVideos in labels.js).

import { MAX_VIDEOS, youtubeID } from './labels.js';
import { el, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// openVideoUpload is "Загрузить видео": the link of the video; onDone gets
// its ID.
function openVideoUpload(onDone) {
  const modal = openModal(tr('Загрузить видео'));
  const error = el('div', { className: 'error', hidden: true });
  const link = el('input', { type: 'text', name: 'link', ariaLabel: tr('Ссылка на YouTube:'), placeholder: 'https://www.youtube.com/watch?v=9FWSRQEqhKE' });

  function upload() {
    const id = youtubeID(link.value);
    if (!id) {
      showError(error, tr('Это не ссылка на видео YouTube.'));
      link.focus();
      return;
    }

    hideError(error);
    modal.close();
    onDone(id);
  }

  modal.box.classList.add('video-upload');
  modal.body.append(
    el('div', { className: 'mb-tabs' }, el('a', { className: 'mb-tab active', textContent: tr('Добавить с YouTube') })),
    error,
    el('table', { className: 'video-upload-form' }, el('tbody', {},
      el('tr', {},
        el('td', { className: 'video-upload-label' }, el('span', { className: 'nobold', textContent: tr('Ссылка на YouTube:') })),
        el('td', {}, link)))),
  );
  modal.actions.append(
    button(tr('Закрыть'), modal.close, { secondary: true }),
    button(tr('Загрузить'), upload),
  );
  link.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      upload();
    }
  });
  link.focus();
}

// videoNode is a video of a message, openvk's msg-attach-j-video.
export function videoNode(id) {
  return el('div', { className: 'msg-attach-j msg-attach-j-video' },
    el('a', {
      className: 'compact_video',
      href: `https://www.youtube.com/watch?v=${id}`,
      ariaLabel: tr('Видеозапись YouTube'),
      onclick: (event) => {
        event.preventDefault();
        playVideo(id);
      },
    },
    el('span', { className: 'play-button', ariaHidden: 'true' }, el('span', { className: 'play-button-ico' }))));
}

// playVideo plays a video in a window, in YouTube's player.
function playVideo(id) {
  const modal = openModal(tr('Видеозапись'));
  modal.box.classList.add('video-player');
  modal.body.append(el('p', { textContent: tr('Если загрузить плеер, браузер подключится к YouTube.') }));
  const load = button(tr('Загрузить плеер YouTube'), () => {
    const frame = el('iframe', {
      className: 'video-frame',
      src: `https://www.youtube-nocookie.com/embed/${id}?autoplay=1`,
      allow: 'autoplay; encrypted-media; picture-in-picture; fullscreen',
      // YouTube refuses to play for a page that sends no origin (error 153),
      // and the pages' Referrer-Policy is same-origin: the frame names it.
      referrerPolicy: 'strict-origin-when-cross-origin',
      allowFullscreen: true,
      title: tr('Видеозапись YouTube'),
    });
    modal.body.replaceChildren(frame);
    load.remove();
    close.focus();
  });
  const close = button(tr('Закрыть'), modal.close, { secondary: true });
  modal.actions.append(close, load);
}

// initVideos wires "Видеозапись" of a form: link opens "Загрузить видео",
// list shows the videos attached, each with a cross.
export function initVideos({ link, list }) {
  let videos = [];

  function render() {
    list.replaceChildren(...videos.map((id) => el('div', { className: 'video-attached-item' },
      el('span', { textContent: 'YouTube' }),
      el('button', { type: 'button', className: 'video-attached-remove', ariaLabel: tr('Не прикреплять'), title: tr('Не прикреплять'), onclick: () => remove(id) },
        el('span', { className: 'cross', ariaHidden: 'true' })))));
    list.hidden = videos.length === 0;
    link.hidden = videos.length >= MAX_VIDEOS;
  }

  function remove(id) {
    videos = videos.filter((v) => v !== id);
    render();
  }

  link.addEventListener('click', () => openVideoUpload((id) => {
    if (!videos.includes(id) && videos.length < MAX_VIDEOS) videos.push(id);
    render();
  }));
  render();

  return {
    ids: () => [...videos],
    count: () => videos.length,
    clear() {
      videos = [];
      render();
    },
  };
}
