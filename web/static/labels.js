// Russian texts for API values and errors, in the language of the page
// (i18n.js): the grammar of English is here too.
import { lang, tr } from './i18n.js';

// genders are "Пол" of the profile form. The site has no pronouns: the
// gender is asked only because Russian words agree with it. openvk's
// "они/их" (neutral) is no longer offered, and a page that chose it is
// spoken of as one whose gender is not set.
export const genders = [
  ['', tr('не указан')],
  ['male', tr('мужской')],
  ['female', tr('женский')],
];

// Relationship labels agree with gender, as in Russian they must; '' is
// for a gender not set.
const relationshipLabels = {
  single: { male: tr('не женат'), female: tr('не замужем'), '': tr('не женат / не замужем') },
  dating: { male: tr('встречается'), female: tr('встречается'), '': tr('встречается') },
  engaged: { male: tr('помолвлен'), female: tr('помолвлена'), '': tr('помолвлен(а)') },
  married: { male: tr('женат'), female: tr('замужем'), '': tr('в браке') },
  civil_union: { male: tr('в гражданском браке'), female: tr('в гражданском браке'), '': tr('в гражданском браке') },
  in_love: { male: tr('влюблён'), female: tr('влюблена'), '': tr('влюблён(а)') },
  complicated: { male: tr('всё сложно'), female: tr('всё сложно'), '': tr('всё сложно') },
  looking: { male: tr('в активном поиске'), female: tr('в активном поиске'), '': tr('в активном поиске') },
};

export function relationships(gender) {
  return [['', tr('не выбрано')], ...Object.entries(relationshipLabels).map(([value, byGender]) => [value, byGender[gender] ?? byGender['']])];
}

// i18n: off — Russian grammar, not text to show.
// partnerPrefixes are openvk's "relationship_N_prefix": the word before
// whom a relationship names, "Женат на …", "Замужем за …".
const partnerPrefixes = {
  dating: 'с', engaged: 'с', civil_union: 'с', complicated: 'с', in_love: 'в',
  married: { male: 'на', female: 'за', '': 'с' },
};
// i18n: on

// takesPartner: the relationship names someone, as in the user service.
export function takesPartner(relationship) {
  return relationship in partnerPrefixes;
}

function russianPrefix(relationship, gender) {
  const prefix = partnerPrefixes[relationship];
  return typeof prefix === 'string' ? prefix : prefix[gender] ?? prefix[''];
}

// englishPrefixes: "married to", "in love with".
const englishPrefixes = { engaged: 'to', married: 'to' };

export function partnerPrefix(relationship, gender) {
  if (lang === 'en') return englishPrefixes[relationship] ?? 'with';
  return russianPrefix(relationship, gender);
}

// The case each of the prefixes asks of the name after it.
const prefixCases = { с: 'ins', за: 'ins', на: 'pre', в: 'acc' };

// partnerName is the partner after the prefix: "замужем за Петром
// Петровым", "женат на Анне Ивановой", "влюблена в Петра Петрова". openvk
// leaves the name as it is.
export function partnerName(relationship, gender, partner) {
  return declinedName(partner, prefixCases[russianPrefix(relationship, gender)]);
}

export const worldviews = [
  ['', tr('не выбрано')],
  ['orthodox', tr('православие')],
  ['catholicism', tr('католицизм')],
  ['protestantism', tr('протестантизм')],
  ['islam', tr('ислам')],
  ['judaism', tr('иудаизм')],
  ['buddhism', tr('буддизм')],
  ['confucianism', tr('конфуцианство')],
  ['secular_humanism', tr('светский гуманизм')],
];

export const politicalViews = [
  ['', tr('не выбраны')],
  ['indifferent', tr('индифферентные')],
  ['communist', tr('коммунистические')],
  ['socialist', tr('социалистические')],
  ['liberal', tr('либеральные')],
  ['conservative', tr('консервативные')],
  ['monarchist', tr('монархические')],
  ['ultraconservative', tr('ультраконсервативные')],
  ['libertarian', tr('либертарианские')],
  ['moderate_islamism', tr('умеренный исламизм')],
  ['radical_islamism', tr('радикальный исламизм')],
];

// wallWrites is "Кто может писать у меня на стене" of openvk. There are no
// friends here: openvk's "Друзья" are the people you follow.
export const wallWrites = [
  ['everyone', tr('все желающие')],
  ['following', tr('те, на кого я подписан(а)')],
  ['only_me', tr('только я')],
];

// messagesWrites is "Кто может писать мне сообщения" of openvk, with its
// "Друзья" as the people you follow; whoever you wrote to may answer.
// chatsInvites are "Кто может добавлять меня в чаты" of openvk.
export const chatsInvites = [
  ['everyone', tr('все желающие')],
  ['following', tr('те, на кого я подписан(а)')],
  ['nobody', tr('никто')],
];

// profileTypes is "Тип профиля" of openvk.
export const profileTypes = [
  ['open', tr('открытый')],
  ['closed', tr('закрытый')],
];

// clubTypes is "Тип группы" of openvk; clubTypeHints is what the page of
// a group says under it.
export const clubTypes = [
  ['open', tr('открытая')],
  ['closed', tr('закрытая')],
];

export const clubTypeHints = {
  open: tr('Это открытая группа. В неё может вступить любой желающий.'),
  closed: tr('Это закрытая группа. Для вступления необходимо подавать заявку.'),
};

// requestsCount is the head of "Заявки" of a closed group, as on openvk.
export function requestsCount(n) {
  if (n <= 0) return tr('Не найдено ни одной заявки...');
  return `${plural(n, ['Найдена', 'Найдено', 'Найдено'])} ${n} ${plural(n, ['заявка', 'заявки', 'заявок'])}`;
}

// requestsLink is the link to them on the page of the group.
export function requestsLink(n) {
  return tr('{0} {1} на вступление', n, plural(n, ['заявка', 'заявки', 'заявок']));
}

export const messagesWrites = [
  ['everyone', tr('все желающие')],
  ['following', tr('те, на кого я подписан(а)')],
  ['nobody', tr('никто')],
];

// audiences are who sees a part of a page ("Кому видно мои группы",
// "… мои фотографии", "… моих друзей", "… основную информацию моей
// страницы" of openvk), with its "Друзьям" as the people you follow.
export const audiences = [
  ['everyone', tr('всем желающим')],
  ['users', tr('пользователям Freenet')],
  ['following', tr('тем, на кого я подписан(а)')],
  ['only_me', tr('только мне')],
];

// pageReads is "Кому в интернете видно мою страницу" of openvk.
export const pageReads = audiences.slice(0, 2);

// onlineReads is "Кто видит мою неактивность" of openvk, in its words.
export const onlineReads = [
  ['everyone', tr('все')],
  ['users', tr('пользователи Freenet')],
  ['following', tr('те, на кого я подписан(а)')],
  ['only_me', tr('никто')],
];

// adminsDisplays is "Список админов" of a community's settings in openvk.
export const adminsDisplays = [
  ['creator', tr('отображать только создателя группы')],
  ['all', tr('отображать всех администраторов')],
  ['none', tr('ничего не отображать')],
];

export const months = [
  tr('января'), tr('февраля'), tr('марта'), tr('апреля'), tr('мая'), tr('июня'),
  tr('июля'), tr('августа'), tr('сентября'), tr('октября'), tr('ноября'), tr('декабря'),
];

// label is the text of a set value; "" for a value that is not set, so the
// profile shows no row instead of the form's "не выбрано".
export function label(options, value) {
  if (!value) return '';
  return options.find(([v]) => v === value)?.[1] ?? '';
}

// plural is the form of a word after the number n: the Russian forms
// for one, a few and many ("голос", "голоса", "голосов"). English has
// two, and they are one text of en.js: "голос|голоса|голосов" is
// "vote|votes" — Russian forms that are alike may be different English.
export function plural(n, forms) {
  if (lang === 'en') {
    const [one, other = one] = tr(forms.join('|')).split('|');
    return n === 1 ? one : other;
  }

  const [one, few, many] = forms;
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

// dayOfMonth is "17 мая", in English "May 17".
function dayOfMonth(day, month) {
  return lang === 'en' ? `${months[month]} ${day}` : `${day} ${months[month]}`;
}

// formatBirthday turns "1990-05-17" into "17 мая 1990 г., 35 лет".
// A birthday shown without its year comes as "--05-17": "17 мая", as
// openvk's "Показывать только день и месяц".
export function formatBirthday(iso) {
  if (iso.startsWith('--')) {
    const [month, day] = iso.slice(2).split('-').map(Number);
    return dayOfMonth(day, month - 1);
  }

  const [year, month, day] = iso.split('-').map(Number);
  const today = new Date();

  let age = today.getFullYear() - year;
  if (today.getMonth() + 1 < month || (today.getMonth() + 1 === month && today.getDate() < day)) age--;

  return tr('{0} {1} г., {2} {3}', dayOfMonth(day, month - 1), year, age, plural(age, ['год', 'года', 'лет']));
}

// The clock dates are read by: null for the browser's own, or minutes
// east of UTC ("Часовой пояс" of the settings).
let timezone = null;

export function setTimezone(minutes) {
  timezone = Number.isInteger(minutes) ? minutes : null;
}

// zoneOffset is how far the clock is ahead of UTC at date, in minutes.
function zoneOffset(date) {
  return timezone ?? -date.getTimezoneOffset();
}

// onClock moves date so that its UTC fields read what the clock shows.
function onClock(date) {
  return new Date(date.getTime() + zoneOffset(date) * 60_000);
}

const two = (n) => String(n).padStart(2, '0');

// formatPostDate turns an ISO time into "сегодня в 9:05", "вчера в 18:30",
// "17 мая в 10:00" or, for other years, "17 мая 2025 в 10:00".
export function formatPostDate(iso, now = new Date()) {
  const date = onClock(new Date(iso));
  const today = onClock(now);
  const time = `${date.getUTCHours()}:${two(date.getUTCMinutes())}`;

  const midnight = (d) => Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate());
  const daysAgo = Math.round((midnight(today) - midnight(date)) / 86_400_000);

  if (daysAgo === 0) return tr('сегодня в {0}', time);
  if (daysAgo === 1) return tr('вчера в {0}', time);

  const day = dayOfMonth(date.getUTCDate(), date.getUTCMonth());

  return date.getUTCFullYear() === today.getUTCFullYear() ? tr('{0} в {1}', day, time) : tr('{0} {1} в {2}', day, date.getUTCFullYear(), time);
}

// storyAge is how long ago a story was told, as VK writes it: "только
// что", "5 минут назад", "3 часа назад" (a story lives a day).
export function storyAge(iso, now = new Date()) {
  const minutes = Math.floor((now - new Date(iso)) / 60_000);

  if (minutes < 1) return tr('только что');
  if (minutes < 60) return tr('{0} {1} назад', minutes, plural(minutes, ['минуту', 'минуты', 'минут']));

  const hours = Math.floor(minutes / 60);

  return tr('{0} {1} назад', hours, plural(hours, ['час', 'часа', 'часов']));
}

// storyViews is "N просмотров" under a story of one's own.
export function storyViews(n) {
  return `${n} ${plural(n, ['просмотр', 'просмотра', 'просмотров'])}`;
}

// storyComments is the link to the comments under a story: "Комментарии"
// or "Комментарии (3)".
export function storyComments(n) {
  return n > 0 ? tr('Комментарии ({0})', n) : tr('Комментировать');
}

// storyViewers heads the box of who viewed a story.
export function storyViewers(n) {
  return tr('Историю {0} {1} {2}', plural(n, ['посмотрел', 'посмотрели', 'посмотрели']), n, plural(n, ['человек', 'человека', 'человек']));
}

// formatDateTime turns an ISO time into openvk's "7 мая 2027 в 12:00".
export function formatDateTime(iso) {
  const date = onClock(new Date(iso));
  const time = `${date.getUTCHours()}:${two(date.getUTCMinutes())}`;

  return tr('{0} {1} в {2}', dayOfMonth(date.getUTCDate(), date.getUTCMonth()), date.getUTCFullYear(), time);
}

// banTerms are how long the admin panel blocks a page for: a number of
// days, months or years from now.
const banTerms = { day: [0, 0, 1], week: [0, 0, 7], month: [0, 1, 0], year: [1, 0, 0] };

// banUntil is when a block chosen in the admin panel ends, as ISO: a term
// from now, '' for good ("forever"); "date" takes the value of a
// datetime-local input, on the visitor's clock ('' if empty).
export function banUntil(term, now = new Date(), value = '') {
  if (term === 'forever') return '';
  if (term === 'date') {
    const [date, time] = value.split('T');
    return localTime(date, time);
  }

  const [years, months, days] = banTerms[term] ?? banTerms.month;
  const at = new Date(now.getTime());
  at.setUTCFullYear(at.getUTCFullYear() + years, at.getUTCMonth() + months, at.getUTCDate() + days);

  return at.toISOString();
}

// foreignAgentText is the words of the label of a foreign agent, which the
// administrators may put on a page (openvk's "user-alert"): the marking of
// a foreign agent's materials, naming the person.
export function foreignAgentText(person) {
  return tr('Данное сообщение (материал) создано и (или) распространено иностранным агентом {0} либо касается деятельности иностранного агента {1}.',
    declinedName(person, 'ins'), genitiveName(person));
}

// banEnd is until when a page is blocked, as openvk's "до: …" or
// "навсегда".
export function banEnd(ban) {
  return ban.until ? tr('до ') + formatDateTime(ban.until) : tr('навсегда');
}

// localTime turns the values of a date and a time input, taken on the
// visitor's clock, into an ISO time; '' unless both are filled in.
export function localTime(date, time) {
  if (!date || !time) return '';

  if (timezone === null) {
    const at = new Date(`${date}T${time}`);
    return Number.isNaN(at.getTime()) ? '' : at.toISOString();
  }

  const at = new Date(`${date}T${time}Z`);
  return Number.isNaN(at.getTime()) ? '' : new Date(at.getTime() - timezone * 60_000).toISOString();
}

// inputDate and inputTime are an ISO time as the values of a date and a
// time input, on the visitor's clock.
export function inputDate(iso) {
  const d = onClock(new Date(iso));

  return `${d.getUTCFullYear()}-${two(d.getUTCMonth() + 1)}-${two(d.getUTCDate())}`;
}

export function inputTime(iso) {
  const d = onClock(new Date(iso));

  return `${two(d.getUTCHours())}:${two(d.getUTCMinutes())}`;
}

// avatarStyles, mainPages and menuItems are the choices of "Интерфейс" of
// the settings, as on openvk; the photos here are square as they are.
export const avatarStyles = [
  ['square', tr('квадратные (по умолчанию)')],
  ['round', tr('круглые')],
];

// themes, ratings, nsfwContents and postViews are the rest of openvk's
// "Интерфейс"; the themes are ours.
export const themes = [
  ['default', tr('обычный (по умолчанию)')],
  ['blue', tr('синий')],
  ['dark', tr('тёмный')],
];

export const ratings = [
  ['show', tr('показывать')],
  ['hide', tr('скрывать')],
];

export const nsfwContents = [
  ['hide', tr('не показывать в ленте')],
  ['blur', tr('только замазывать')],
  ['show', tr('показывать')],
];

export const postViews = [
  ['old', tr('старый')],
  ['microblog', tr('микроблог')],
];

// timezones are openvk's clocks ("Часовой пояс"), by minutes east of
// UTC; the first is the browser's own.
export const timezones = [
  ['', tr('как в браузере (по умолчанию)')],
  ['-720', tr('(GMT-12): Остров Бейкер, Хауленд')],
  ['-660', tr('(GMT-11): Паго-Паго, Ниуэ, Мидуэй')],
  ['-600', tr('(GMT-10): Гонолулу, Папеэте, Адак')],
  ['-540', tr('(GMT-09): Анкоридж, Фэрбанкс, Джуно')],
  ['-480', tr('(GMT-08): Лос-Анджелес, Ванкувер, Тихуана')],
  ['-420', tr('(GMT-07): Денвер, Эдмонтон, Финикс')],
  ['-360', tr('(GMT-06): Мехико, Чикаго, Гватемала')],
  ['-300', tr('(GMT-05): Нью-Йорк, Торонто, Гавана')],
  ['-270', tr('(GMT-04:30): Каракас, Маракайбо')],
  ['-240', tr('(GMT-04): Сантьяго, Ла-Пас, Галифакс')],
  ['-210', tr('(GMT-03:30): Сент-Джонс (Ньюфаундленд)')],
  ['-180', tr('(GMT-03): Буэнос-Айрес, Сан-Паулу, Монтевидео')],
  ['-120', tr('(GMT-02): Южная Георгия')],
  ['-60', tr('(GMT-01): Азорские острова, Прая')],
  ['0', tr('(GMT): Лондон, Лиссабон, Касабланка')],
  ['60', tr('(GMT+1): Берлин, Париж, Рим')],
  ['120', tr('(GMT+2): Калининград (МСК-1), Киев, Каир')],
  ['180', tr('(GMT+3): Москва, Стамбул, Минск')],
  ['210', tr('(GMT+3:30): Тегеран, Исфахан')],
  ['240', tr('(GMT+4): Самара (МСК+1), Дубай, Баку')],
  ['270', tr('(GMT+4:30): Кабул, Кандагар')],
  ['300', tr('(GMT+5): Екатеринбург (МСК+2), Ташкент, Карачи')],
  ['330', tr('(GMT+5:30): Нью-Дели, Мумбаи, Коломбо')],
  ['345', tr('(GMT+5:45): Катманду, Покхара')],
  ['360', tr('(GMT+6): Омск (МСК+3), Алматы, Дакка')],
  ['390', tr('(GMT+6:30): Янгон (Рангун), Нейпьидо')],
  ['420', tr('(GMT+7): Новосибирск (МСК+4), Красноярск, Бангкок')],
  ['480', tr('(GMT+8): Иркутск (МСК+5), Пекин, Сингапур')],
  ['510', tr('(GMT+8:30): Пхеньян')],
  ['540', tr('(GMT+9): Якутск (МСК+6), Токио, Сеул')],
  ['570', tr('(GMT+9:30): Аделаида, Дарвин')],
  ['600', tr('(GMT+10): Владивосток (МСК+7), Сидней, Мельбурн')],
  ['630', tr('(GMT+10:30): Лорд-Хау')],
  ['660', tr('(GMT+11): Магадан (МСК+8), Сахалин, Чокурдах')],
  ['720', tr('(GMT+12): Камчатка (МСК+9), Анадырь, Окленд')],
  ['765', tr('(GMT+12:45): Острова Чатем')],
  ['780', tr('(GMT+13): Нукуалофа, Апиа')],
  ['840', tr('(GMT+14): Киритимати (о-ва Лайн)')],
];

export const mainPages = [
  ['page', tr('страница')],
  ['feed', tr('новости')],
];

export const menuItems = [
  ['photos', tr('Фотографии')],
  ['messages', tr('Сообщения')],
  ['groups', tr('Группы')],
  ['feed', tr('Новости')],
  ['notifications', tr('Уведомления')],
];

// deactivationReasons are openvk's reasons to delete a page: a title and
// the words it puts in the message; the last one leaves them to the user.
export const deactivationReasons = [
  [tr('У меня есть другая страница на сайте'), tr('Я создал новую страницу и теперь хочу подтереть своё прошлое.')],
  [tr('Сайт отнимает у меня слишком много времени'), tr('Пусть этот сайт хорош и прекрасен, но он отнимает у меня время, которое мне нужно для работы и жизни.')],
  [tr('Сайт содержит слишком много неприемлемых материалов'), tr('Я нашёл достаточно порнографии и пиратского контента - хватит на всю жизнь. Теперь я ухожу.')],
  [tr('Меня беспокоит безопасность моих данных'), tr('За мной следят и мне страшно здесь находится. Извините, я вынужден уйти.')],
  [tr('Мою страницу не комментируют'), tr('Меня никто не смотрит здесь и это грустно. Вы пожалеете о том, что я ушёл.')],
  [tr('Другая причина'), ''],
];

// byGender picks the form of a verb for a person; the masculine speaks of
// one whose gender is not set, as on openvk.
function byGender(gender, male, female) {
  return gender === 'female' ? female : male;
}

export function wrote(gender) {
  return byGender(gender, tr('написал'), tr('написала'));
}

// limitedAccess is openvk's "$1 ограничил доступ к своей странице." for
// whoever blacklisted the viewer.
export function limitedAccess(gender) {
  return { male: tr('ограничил'), female: tr('ограничила') }[gender] ?? tr('ограничил(а)');
}

// closedFollows says whom the owner of a closed profile lets in.
export function closedFollows(gender) {
  return { male: tr('подписан сам'), female: tr('подписана сама') }[gender] ?? tr('подписан(а)');
}

// blacklistCount is the head of the blacklist, as on openvk.
export function blacklistCount(n) {
  if (n <= 0) return tr('В вашем чёрном списке ещё нет пользователей.');
  return tr('В вашем чёрном списке {0} {1}.', n, plural(n, ['пользователь', 'пользователя', 'пользователей']));
}

// sessionsCount heads the list of sessions in the settings.
export function sessionsCount(n) {
  return tr('Вы вошли на сайт с {0} {1}.', n, plural(n, ['устройства', 'устройств', 'устройств']));
}

const browsers = [
  [/YaBrowser\//, tr('Яндекс Браузер')],
  [/Edg(e|A|iOS)?\//, 'Edge'],
  [/OPR\/|Opera/, 'Opera'],
  [/Firefox\/|FxiOS\//, 'Firefox'],
  [/Chrome\/|CriOS\//, 'Chrome'],
  [/Version\/.*Safari\//, 'Safari'],
];

const systems = [
  [/Android/, 'Android'],
  [/iPhone/, 'iPhone'],
  [/iPad/, 'iPad'],
  [/Windows/, 'Windows'],
  [/CrOS/, 'ChromeOS'],
  [/Mac OS X|Macintosh/, 'macOS'],
  [/Linux/, 'Linux'],
];

// deviceName says what a session is used from by what its browser called
// itself: "Chrome, Windows". The order of the lists matters: Edge and
// Opera call themselves Chrome too, and Android calls itself Linux. A name
// that says nothing we know is shown as it is, cut short.
export function deviceName(userAgent) {
  const agent = (userAgent ?? '').trim();
  if (!agent) return tr('Неизвестное устройство');

  const known = [browsers, systems]
    .map((list) => list.find(([pattern]) => pattern.test(agent))?.[1])
    .filter(Boolean);
  if (known.length) return known.join(', ');

  return agent.length > 60 ? agent.slice(0, 60) + '…' : agent;
}

// declinedName is the name of a person in a case of Russian: 'gen' —
// "на стене Петра Петрова", as openvk says it, 'acc' — "пригласила Анну
// Иванову", 'ins' — "замужем за Петром Петровым", 'pre' — "женат на Анне
// Ивановой". Names are declined by their endings; whatever the rules do
// not know (Latin letters, Шевченко, Кармен) stays as it is.
export function declinedName({ name = '', surname = '', gender = '' }, grammaticalCase) {
  const at = lang === 'en' ? -1 : CASES.indexOf(grammaticalCase);

  return [
    decline(name, firstNameParadigm(name, gender), at),
    decline(surname, surnameParadigm(surname, gender), at),
  ].filter(Boolean).join(' ');
}

export function genitiveName(person) {
  return declinedName(person, 'gen');
}

// i18n: off — Russian grammar, not text to show.
// The order of the endings in a paradigm.
const CASES = ['gen', 'acc', 'ins', 'pre'];

const CYRILLIC = /^[а-яё-]+$/i;
const VOWELS = /[аеёиоуыэюя]/gi;
// After these letters Russian writes и, not ы: Ольга → Ольги.
const SOFT_BEFORE_A = /[гкхжчшщ]а$/i;
// Names that lose a vowel: Павел → Павла.
const fleetingNames = { павел: 'Павл', лев: 'Льв', пётр: 'Петр', петр: 'Петр' };
// Women's names that end with ь like men's: Любовь → Любови, but Игорь →
// Игоря. They tell themselves apart when gender is not set.
const softFemaleNames = new Set(['любовь', 'нинель', 'адель', 'эсфирь', 'юдифь', 'руфь', 'рахиль', 'гузель', 'айгуль']);
// Илья is stressed at the end: Ильёй, not Ильей.
const stressedNames = new Set(['илья']);
// Surnames that look like those losing a vowel and keep it: Игрока.
const steadySurnames = new Set(['игрок', 'знаток', 'ездок', 'седок', 'едок', 'ходок', 'пророк', 'восток', 'поток', 'чеснок', 'урок', 'порок', 'зарок', 'брелок']);
const fleetingSurnames = { заяц: 'Зайц' };

const MALE_NOUN = ['а', 'а', 'ом', 'е'];

// decline puts a word in the case at; a paradigm is how many letters the
// word loses, what it gets instead of them and its endings, and none
// leaves the word as it is. A word written in capitals stays so.
function decline(word, paradigm, at) {
  if (!paradigm || at < 0) return word;

  const [lost, stem, endings] = paradigm;
  const declined = (lost > 0 ? word.slice(0, -lost) : word) + stem + endings[at];

  return word === word.toUpperCase() ? declined.toUpperCase() : declined;
}

function firstNameParadigm(name, gender) {
  const lower = name.toLowerCase();
  if (!CYRILLIC.test(name)) return null;
  if (softFemaleNames.has(lower)) gender = 'female';

  if (gender !== 'female') {
    const fleeting = fleetingNames[lower];
    if (fleeting) return [name.length, sameCase(name, fleeting), MALE_NOUN];
    // Витёк → Витька.
    if (/[^аеёиоуыэюя]ёк$/i.test(name)) return [2, 'ьк', MALE_NOUN];
  }

  return nounParadigm(name, gender);
}

function surnameParadigm(surname, gender) {
  const lower = surname.toLowerCase();
  if (!CYRILLIC.test(surname)) return null;

  // Female forms tell themselves apart even when gender is not set.
  if (/(ов|ев|ёв|ин|ын)а$/i.test(surname)) return [1, '', ['ой', 'у', 'ой', 'ой']];
  if (/[жчшщ]ая$/i.test(surname)) return [2, '', ['ей', 'ую', 'ей', 'ей']];
  if (/ая$/i.test(surname)) return [2, '', ['ой', 'ую', 'ой', 'ой']];
  if (/яя$/i.test(surname)) return [2, '', ['ей', 'юю', 'ей', 'ей']];
  // A woman's surname is declined only when it ends like a noun of hers:
  // Сковорода → Сковороды, but Шевчук stays.
  if (gender === 'female') return /[ая]$/i.test(surname) ? nounParadigm(surname, gender) : null;

  if (/(ых|их)$/i.test(surname)) return null;
  // Цой → Цоя: a short surname is a noun, not an adjective.
  const adjective = (surname.match(VOWELS) ?? []).length > 1;
  if (adjective && /[гкх]ий$/i.test(surname)) return [2, '', ['ого', 'ого', 'им', 'ом']];
  if (adjective && /[жчшщн]ий$/i.test(surname)) return [2, '', ['его', 'его', 'им', 'ем']];
  if (adjective && /[гкхжчшщ]ой$/i.test(surname)) return [2, '', ['ого', 'ого', 'им', 'ом']];
  if (adjective && /(ый|ой)$/i.test(surname)) return [2, '', ['ого', 'ого', 'ым', 'ом']];
  if (/(ов|ев|ёв|ин|ын)$/i.test(surname)) return [0, '', ['а', 'а', 'ым', 'е']];

  return fleetingSurnameParadigm(surname, lower) ?? nounParadigm(surname, gender);
}

// fleetingSurnameParadigm is of the surnames that lose their last vowel:
// Кравец → Кравца, Стрелец → Стрельца, Коломиец → Коломийца, Волчок →
// Волчка, Королёк → Королька. After two consonants the vowel stays
// (Кузнец → Кузнеца), as it does in a word of one syllable (Швец, Блок).
function fleetingSurnameParadigm(surname, lower) {
  const syllables = (surname.match(VOWELS) ?? []).length;
  if (fleetingSurnames[lower]) return [surname.length, sameCase(surname, fleetingSurnames[lower]), ['а', 'а', 'ем', 'е']];
  if (syllables < 2 || steadySurnames.has(lower)) return null;

  // The ending is stressed in the short ones: Кравцом, but Гриневцем.
  const afterTs = ['а', 'а', syllables > 2 ? 'ем' : 'ом', 'е'];
  if (/[аеёиоуыэюя]ец$/i.test(surname)) return [2, 'йц', afterTs];
  if (/[аеёиоуыэюя]лец$/i.test(surname)) return [2, 'ьц', afterTs];
  if (/[аеёиоуыэюя][бвгджзкмнпрстфхчшщ]ец$/i.test(surname)) return [2, 'ц', afterTs];
  if (/[аеёиоуыэюя]ёк$/i.test(surname)) return [2, 'йк', MALE_NOUN];
  if (/ёк$/i.test(surname)) return [2, 'ьк', MALE_NOUN];
  if (/[^аеёиоуыэюя]ок$/i.test(surname)) return [2, 'к', MALE_NOUN];

  return null;
}

// nounParadigm declines a noun-like name or surname by its last letters.
function nounParadigm(word, gender) {
  if (/ия$/i.test(word)) return [1, '', ['и', 'ю', 'ей', 'и']];
  if (/я$/i.test(word)) return [1, '', ['и', 'ю', stressedNames.has(word.toLowerCase()) ? 'ёй' : 'ей', 'е']];
  // Моруа, Галуа.
  if (/[аеёиоуыэюя]а$/i.test(word)) return null;
  if (/а$/i.test(word)) return [1, '', [SOFT_BEFORE_A.test(word) ? 'и' : 'ы', 'у', /[жчшщц]а$/i.test(word) ? 'ей' : 'ой', 'е']];
  if (/[оеёиуюэы]$/i.test(word)) return null;
  if (gender === 'female') return /ь$/i.test(word) ? [1, '', ['и', 'ь', 'ью', 'и']] : null;
  if (/ий$/i.test(word)) return [1, '', ['я', 'я', 'ем', 'и']];
  if (/[йь]$/i.test(word)) return [1, '', ['я', 'я', 'ем', 'е']];

  return [0, '', ['а', 'а', /[жчшщ]$/i.test(word) ? 'ем' : 'ом', 'е']];
}
// i18n: on

// sameCase writes a stem in capitals when the word it is of is.
function sameCase(word, stem) {
  return word === word.toUpperCase() ? stem.toUpperCase() : stem;
}

export function followersCount(n) {
  return `${n} ${plural(n, ['подписчик', 'подписчика', 'подписчиков'])}`;
}

// presence is openvk's "Онлайн" or "была в сети сегодня в 12:00" of a
// page, '' when it is not told.
export function presence(user, now = new Date()) {
  if (user.online) return tr('Онлайн');
  if (!user.lastSeenAt) return '';

  return tr('{0} в сети {1}', byGender(user.gender, tr('был'), tr('была')), formatPostDate(user.lastSeenAt, now));
}

export function followingCount(n) {
  return `${n} ${plural(n, ['подписка', 'подписки', 'подписок'])}`;
}

// membersCount is openvk's "participants": "Ни одного участника", "3 участника".
export function membersCount(n) {
  return n === 0 ? tr('Ни одного участника') : `${n} ${plural(n, ['участник', 'участника', 'участников'])}`;
}

// myClubsCount is the head of "Мои Группы", openvk's "groups_list".
export function myClubsCount(n) {
  if (n === 0) return tr('Вы не состоите ни в одной группе');
  return tr('Вы состоите в {0} {1}', n, plural(n, ['группе', 'группах', 'группах']));
}

// otherComments is openvk's "view_other_comments" under a post of
// "Микроблог": how many comments are on the page of the post only.
export function otherComments(n) {
  return tr('Показать остальные комментарии ({0})', n);
}

// aboutStatistics are the lines of "Статистика" on the page "о сайте",
// openvk's "about_users" and so on: the number and the words after it.
export function aboutStatistics({ users, online, clubs, posts }) {
  return [
    [users, plural(users, ['пользователь', 'пользователя', 'пользователей'])],
    [online, plural(online, ['пользователь в сети', 'пользователя в сети', 'пользователей в сети'])],
    [clubs, plural(clubs, ['группа', 'группы', 'групп'])],
    [posts, plural(posts, ['запись на стенах', 'записи на стенах', 'записей на стенах'])],
  ];
}

// adminOverview are the words under the numbers of "Сводка" of the
// admin panel, openvk's "admin_overview_*": the users with the list of
// those online and the verified ones under them, the groups and the posts.
export function adminOverview({ users, verified = 0, clubs, posts }) {
  return {
    users: plural(users, ['зарегистрированный пользователь:', 'зарегистрированных пользователя, из них:', 'зарегистрированных пользователей, из них:']),
    online: tr('онлайн'),
    verified: plural(verified, ['верифицированный', 'верифицированных', 'верифицированных']),
    clubs: plural(clubs, ['группа', 'группы', 'групп']),
    posts: plural(posts, ['запись', 'записи', 'записей']),
  };
}

// clubsCount is openvk's "groups": "1 группа", "5 групп".
export function clubsCount(n) {
  return n === 0 ? tr('Ни одной группы') : `${n} ${plural(n, ['группа', 'группы', 'групп'])}`;
}

// adminsCount is openvk's "administrators": "1 администратор", "5 администраторов".
export function adminsCount(n) {
  return `${n} ${plural(n, ['администратор', 'администратора', 'администраторов'])}`;
}

// suggestedByEveryone and suggestedByYou are the links above the wall of
// a "Предложка", openvk's "suggested_by_everyone" and "suggested_by_you".
export function suggestedByEveryone(n) {
  return `${n} ${plural(n, ['предложенная запись', 'предложенные записи', 'предложенных записей'])}`;
}

export function suggestedByYou(n) {
  return `${n} ${plural(n, ['предложенная вами запись', 'предложенные вами записи', 'предложенных вами записей'])}`;
}

// postponedCount is VK's link above the wall: "3 отложенные записи".
export function postponedCount(n) {
  if (n === 0) return tr('Нет отложенных записей');

  return `${n} ${plural(n, ['отложенная запись', 'отложенные записи', 'отложенных записей'])}`;
}

// publishesAt is when a postponed post comes out, as VK says it.
export function publishesAt(iso) {
  return tr('Будет опубликована ') + formatDateTime(iso);
}

// nextHour is the time "Таймер" offers first: the next full hour.
export function nextHour(now = new Date()) {
  const at = new Date(now.getTime() + 3_600_000);
  at.setUTCMinutes(0, 0, 0);

  return at.toISOString();
}

// topicsCount is openvk's "topics": "1 тема", "5 тем".
export function topicsCount(n) {
  return `${n} ${plural(n, ['тема', 'темы', 'тем'])}`;
}

// topicMessages is under a topic in the list, openvk's "messages".
export function topicMessages(n) {
  return `${n} ${plural(n, ['сообщение', 'сообщения', 'сообщений'])}`;
}

// topicMessagesCount heads a topic, openvk's "topic_messages_count".
export function topicMessagesCount(n) {
  return n === 0 ? tr('В теме нет сообщений') : tr('В теме {0}', topicMessages(n));
}

// resultsCount is openvk's "results": "Ни одного результата", "2 результата".
export function resultsCount(n) {
  return n === 0 ? tr('Ни одного результата') : `${n} ${plural(n, ['результат', 'результата', 'результатов'])}`;
}

// suggestedInClub is the head of the suggestions of a club, openvk's
// "suggested_posts_in_group" and, for your own, "..._by_you"; at 0, once
// all are handled.
export function suggestedInClub(n, mine) {
  if (n === 0) return mine ? tr('Вы не предлагали в эту группу никаких записей') : tr('Вы посмотрели всю предложку, поздравляю!');

  const posts = `${n} ${plural(n, ['запись', 'записи', 'записей'])}`;
  return mine ? tr('Вы предложили в эту группу {0}', posts) : tr('В эту группу предложили {0}', posts);
}

// albumsCount is openvk's "albums": "Ни одного альбома", "3 альбома".
export function albumsCount(n) {
  return n === 0 ? tr('Ни одного альбома') : `${n} ${plural(n, ['альбом', 'альбома', 'альбомов'])}`;
}

// myAlbumsCount is the head of your own albums, openvk's "albums_list".
export function myAlbumsCount(n) {
  return n === 0 ? tr('У Вас нет ни одного альбома') : tr('У Вас {0}', albumsCount(n));
}

// Gifts ("Подарки" of openvk): the collections there are to choose from,
// what each gift is called and its picture. The gifts themselves are the
// catalogue of the user service; a picture is a file of the site, named
// by its whole path so that it is served with its version.
export const giftCollections = { flowers: tr('Цветы'), sweets: tr('Сладости'), holidays: tr('Праздники'), freenet: 'Freenet' };

export const giftNames = {
  rose: tr('Роза'), tulip: tr('Тюльпан'), bouquet: tr('Букет'), sunflower: tr('Подсолнух'), cactus: tr('Кактус'),
  cake: tr('Торт'), candy: tr('Конфета'), chocolate: tr('Шоколад'), icecream: tr('Мороженое'), cupcake: tr('Пирожное'),
  present: tr('Подарок'), balloons: tr('Шарики'), heart: tr('Сердце'), bear: tr('Мишка'), star: tr('Звезда'),
  freenet_crystal: tr('Кристалл Freenet'), freenet_coin: tr('Золотая монета'), freenet_ring: tr('Перстень'), freenet_crown: tr('Корона'), freenet_heart: tr('Рубиновое сердце'), freenet_cup: tr('Кубок'),
};

const giftImages = {
  freenet_crystal: '/static/gifts/freenet_crystal.png',
  freenet_coin: '/static/gifts/freenet_coin.png',
  freenet_ring: '/static/gifts/freenet_ring.png',
  freenet_crown: '/static/gifts/freenet_crown.png',
  freenet_heart: '/static/gifts/freenet_heart.png',
  freenet_cup: '/static/gifts/freenet_cup.png',
  rose: '/static/gifts/rose.svg',
  tulip: '/static/gifts/tulip.svg',
  bouquet: '/static/gifts/bouquet.svg',
  sunflower: '/static/gifts/sunflower.svg',
  cactus: '/static/gifts/cactus.svg',
  cake: '/static/gifts/cake.svg',
  candy: '/static/gifts/candy.svg',
  chocolate: '/static/gifts/chocolate.svg',
  icecream: '/static/gifts/icecream.svg',
  cupcake: '/static/gifts/cupcake.svg',
  present: '/static/gifts/present.svg',
  balloons: '/static/gifts/balloons.svg',
  heart: '/static/gifts/heart.svg',
  bear: '/static/gifts/bear.svg',
  star: '/static/gifts/star.svg',
};

// giftImage is the picture of a gift; a gift this site does not know yet
// (a newer catalogue) is shown as a present.
export function giftImage(gift) {
  return giftImages[gift] ?? giftImages.present;
}

// Stickers ("Стикеры" of openvk): what each pack is called and the
// picture of each sticker. The packs are the catalogue of the user
// service; a picture is a file of the site, named by its whole path.
export const stickerPacks = { smiles: tr('Смайлы'), cats: tr('Котики'), bears: tr('Мишки') };

const stickerImages = {
  smile_happy: '/static/stickers/smile_happy.svg',
  smile_laugh: '/static/stickers/smile_laugh.svg',
  smile_wink: '/static/stickers/smile_wink.svg',
  smile_love: '/static/stickers/smile_love.svg',
  smile_cool: '/static/stickers/smile_cool.svg',
  smile_wow: '/static/stickers/smile_wow.svg',
  smile_sad: '/static/stickers/smile_sad.svg',
  smile_angry: '/static/stickers/smile_angry.svg',
  cat_hi: '/static/stickers/cat_hi.svg',
  cat_love: '/static/stickers/cat_love.svg',
  cat_lol: '/static/stickers/cat_lol.svg',
  cat_wow: '/static/stickers/cat_wow.svg',
  cat_sleep: '/static/stickers/cat_sleep.svg',
  cat_angry: '/static/stickers/cat_angry.svg',
  bear_hi: '/static/stickers/bear_hi.svg',
  bear_hug: '/static/stickers/bear_hug.svg',
  bear_love: '/static/stickers/bear_love.svg',
  bear_ok: '/static/stickers/bear_ok.svg',
  bear_party: '/static/stickers/bear_party.svg',
  bear_sad: '/static/stickers/bear_sad.svg',
};

// A sticker of a pack a person made is the ID of its photo in 32 hex
// digits; the gateway serves its picture.
const photoSticker = /^[0-9a-f]{32}$/;

// stickerImage is the picture of a sticker; null for one this site does
// not know yet (a newer catalogue).
export function stickerImage(sticker) {
  if (photoSticker.test(sticker)) return `/media/stickers/${sticker}`;

  return stickerImages[sticker] ?? null;
}

// stickerPackName is what a pack is called: its author named a pack made
// of photos, the site names its own.
export function stickerPackName(pack) {
  return pack.name || stickerPacks[pack.id] || tr('Стикеры');
}

// stickerPackAction is the button of a pack in openvk's box "Информация о
// наборе": a pack once bought and put away comes back free.
export function stickerPackAction(pack) {
  if (pack.added) return tr('Приобретён');
  if (pack.bought || !pack.price) return tr('Добавить');

  return tr('Купить за {0}', votesCount(pack.price));
}

// stickerPackPrice is the foot of a card of the shop, as openvk's: text
// and its kind — "purchased" grey, "free" green, "paid" blue.
export function stickerPackPrice(pack) {
  if (pack.added) return { text: tr('Приобретён'), kind: 'purchased' };
  if (pack.bought) return { text: tr('Добавить'), kind: 'free' };
  if (pack.price) return { text: votesCount(pack.price), kind: 'paid' };

  return { text: tr('Бесплатно'), kind: 'free' };
}

// STICKER_SECTIONS are the sections of openvk's shop.
export const STICKER_SECTIONS = { popular: tr('Популярные'), free: tr('Бесплатные'), all: tr('Все') };

// shopPacks are the packs of a section of the shop, as openvk's: "Популярные"
// — those with a price, "Бесплатные" — the free ones, both the most taken
// first and then the newest, "Все" — all, the oldest first (those of the
// site, then those of people, whose names are UUIDv7). query keeps those
// whose name, description or author has it, in any case.
export function shopPacks(packs, section, query = '') {
  const q = query.trim().toLocaleLowerCase('ru');
  const found = packs.filter((p) => !q || [stickerPackName(p), p.about ?? '', p.author ? `${p.author.name} ${p.author.surname}` : '']
    .some((s) => s.toLocaleLowerCase('ru').includes(q)));
  const site = (p) => (p.author || p.name ? 1 : 0);
  const older = (a, b) => site(a) - site(b) || (site(a) ? a.id.localeCompare(b.id) : 0);

  if (section === 'all') return [...found].sort(older);

  return found
    .filter((p) => (section === 'free' ? !p.price : p.price > 0))
    .sort((a, b) => (b.holders ?? 0) - (a.holders ?? 0) || older(b, a));
}

// The global feed shows an ad after its third post and then after every
// eighth, as Twitter slips one promoted tweet among about ten.
const AD_FIRST = 3;
const AD_EVERY = 8;

// adDue tells whether an ad follows the shown-th post of the feed, from 1.
export function adDue(shown) {
  return shown >= AD_FIRST && (shown - AD_FIRST) % AD_EVERY === 0;
}

// peopleCount is "1 человек", "3 человека", "5 человек".
export function peopleCount(n) {
  return `${n} ${plural(n, ['человек', 'человека', 'человек'])}`;
}

// adRate is the share of the views of an ad that were followed, as the
// stats of the admin panel say it: "4,2 %"; "—" before any view.
export function adRate({ views, clicks }) {
  if (!views) return '—';

  const percent = String(Math.round((clicks / views) * 1000) / 10);

  return `${lang === 'en' ? percent : percent.replace('.', ',')} %`;
}

// share is a share in percents with one decimal, as the admin panel says
// it: "4,2 %".
function share(value) {
  const percent = String(Math.round(value * 10) / 10);

  return `${lang === 'en' ? percent : percent.replace('.', ',')} %`;
}

// casinoChances are the chances of the prizes of the casino, in percents:
// each weight among all weights; none when no prize has a weight.
export function casinoChances(prizes) {
  const weights = prizes.map((p) => Math.max(0, Number(p.weight) || 0));
  const total = weights.reduce((sum, w) => sum + w, 0);

  return weights.map((w) => (total > 0 ? (w * 100) / total : 0));
}

// casinoChance is a chance as the admin panel says it: "12,5 %".
export function casinoChance(percent) {
  return share(percent);
}

// casinoReturn is the share of the votes spent on spins the casino gives
// back, on average by its odds or in fact by its stats: "70,5 %"; "—"
// with nothing to say it from.
export function casinoReturn(price, prizes) {
  const chances = casinoChances(prizes);
  if (!(price > 0) || !chances.some((c) => c > 0)) return '—';

  const won = prizes.reduce((sum, p, i) => sum + (Math.max(0, Number(p.votes) || 0) * chances[i]) / 100, 0);

  return share((won * 100) / price);
}

export function casinoPayout({ spent, won }) {
  return spent > 0 ? share((won * 100) / spent) : '—';
}

// casinoPrize is a prize as the casino shows it: "50 голосов", and
// "Пусто" for nothing.
export function casinoPrize(votes) {
  return votes > 0 ? votesCount(votes) : tr('Пусто');
}

// spinsCount is "1 прокрутка", "5 прокруток".
export function spinsCount(n) {
  return `${n} ${plural(n, ['прокрутка', 'прокрутки', 'прокруток'])}`;
}

// autoplayNext says whether "Автоигра" plays another round: "go", "done"
// once it played the rounds chosen (0 is "пока есть голоса": no end of its
// own), or "votes" when the balance does not pay for the next one.
export function autoplayNext({ played, times, balance, cost }) {
  if (times > 0 && played >= times) return 'done';

  return balance >= cost ? 'go' : 'votes';
}

// votesFontSize is the size, in pixels, of the balance in the box of
// "Голоса" 140 px wide: 48 px, as openvk, while the digits fit, and smaller
// as they grow (a digit of Tahoma, bold too, is under 0,65 of the size), as openvk's
// textFit does.
export function votesFontSize(votes, width = 140) {
  const digits = String(Math.max(0, Math.trunc(votes))).length;

  return Math.max(12, Math.min(48, Math.floor(width / (digits * 0.65))));
}

// wagersCount is "1 ставка", "5 ставок".
export function wagersCount(n) {
  return `${n} ${plural(n, ['ставка', 'ставки', 'ставок'])}`;
}

// betMultiplier is a multiplier of a bet, given in hundredths: "×1,5".
export function betMultiplier(hundredths) {
  const times = String(hundredths / 100);

  return `×${lang === 'en' ? times : times.replace('.', ',')}`;
}

// aimReturn is the share of the votes bet on a multiplier, in hundredths,
// that it gives back on average with its chance of 10000, in percents.
export function aimReturn(hundredths, chance) {
  return (hundredths * chance) / 10000;
}

// betChance is a chance of 10000 as the casino says it: "47,5 %".
export function betChance(odds) {
  return share(odds / 100);
}

// betRoll is a roll of 10000 as a number of 100: "37,21".
export function betRoll(roll) {
  const fixed = (roll / 100).toFixed(2);

  return lang === 'en' ? fixed : fixed.replace('.', ',');
}

// aimZone is the green part of the strip of "Прицел": chance wide, in the
// middle — the rolls from low to high, high not included, of 10000, as the
// user service counts them.
export function aimZone(odds) {
  const low = Math.floor((10000 - odds) / 2);

  return { low, high: low + odds };
}

// betEventShare is the share of all the votes staked on an event that
// stand on one outcome, in percents.
export function betEventShare(staked, total) {
  return total > 0 ? (staked * 100) / total : 0;
}

// betEventOdds is what a vote staked on an outcome brings if it wins: its
// odds, "×2,5", or for an event without them, all staked shared by those
// on it now; '' with nothing on it.
export function betEventOdds(outcome, total) {
  if (outcome.odds > 0) return betMultiplier(outcome.odds);

  return outcome.staked > 0 ? betMultiplier(Math.floor((total * 100) / outcome.staked)) : '';
}

// betOddsField is odds in hundredths as the administrators type them:
// "2,5"; and odds of such a field, 0 for what is not a number.
export function betOddsField(hundredths) {
  const times = String(hundredths / 100);

  return lang === 'en' ? times : times.replace('.', ',');
}

export function typedOdds(value) {
  const times = Number(String(value).trim().replace(',', '.'));

  return Number.isFinite(times) ? Math.round(times * 100) : 0;
}

// betEventState says where an event of "Ставки" stands: whether it takes
// stakes and till when, or how it ended.
export function betEventState(e) {
  if (e.state === 'resolved') return tr('Итог: {0}.', e.outcomes[e.winner]?.name ?? '');
  if (e.state === 'cancelled') return tr('Событие отменено, ставки вернулись.');
  if (!e.open) return tr('Ставки больше не принимаются — ждём итога.');

  return e.closesAt ? tr('Ставки принимаются до {0}.', formatDateTime(e.closesAt)) : tr('Ставки принимаются, пока событие не закроют.');
}

// betPrize is what a bet wins: the votes multiplied, rounded down.
export function betPrize(amount, hundredths) {
  return Math.floor((amount * hundredths) / 100);
}

// betAmount is the bet a quick button makes of amount: the least, half,
// double or the most the casino and the balance allow — always within
// the limits.
export function betAmount(amount, quick, { least, most, balance }) {
  const top = Math.max(least, Math.min(most, balance ?? most));
  const clamp = (n) => Math.min(most, Math.max(least, Math.floor(n) || least));

  switch (quick) {
    case 'least': return least;
    case 'half': return clamp(amount / 2);
    case 'double': return clamp(Math.min(amount * 2, top));
    case 'most': return top;
    default: return clamp(amount);
  }
}

// The multiplier of "Ракетка" grows as e to the power of rocketGrowth
// times the milliseconds of the flight, as the user service counts it
// (user.RocketGrowth): ×2 in about 5.8 s, ×10 in 19 s.
const rocketGrowth = 0.00012;

// rocketMultiplier is the multiplier of a rocket flying ms milliseconds, in
// hundredths, rounded down — at most ×10000.
export function rocketMultiplier(ms) {
  if (!(ms > 0)) return 100;

  return Math.min(1_000_000, Math.floor(100 * Math.exp(rocketGrowth * Math.floor(ms))));
}

// rocketTime is how many milliseconds a rocket flies to reach a multiplier
// in hundredths.
export function rocketTime(hundredths) {
  return hundredths > 100 ? Math.log(hundredths / 100) / rocketGrowth : 0;
}

// rocketShown is the multiplier a rocket shows, always with two digits:
// "×1,05".
export function rocketShown(hundredths) {
  const fixed = (hundredths / 100).toFixed(2);

  return `×${lang === 'en' ? fixed : fixed.replace('.', ',')}`;
}

// rocketsCount is "1 полёт", "5 полётов".
export function rocketsCount(n) {
  return `${n} ${plural(n, ['полёт', 'полёта', 'полётов'])}`;
}

// rocketScale is the view of the sky of "Ракетка" over a flight of ms:
// at least 8 s and ×2, and a little more than the flight, so that the
// rocket stays in sight as it climbs.
export function rocketScale(ms) {
  const time = Math.max(8000, ms * 1.15);

  return { time, top: Math.max(200, rocketMultiplier(ms) * 1.15) };
}

// casinoPlan is the path of the highlight over the cells of the casino
// before it stops on the prize won: from the cell after start, round
// after round, each step a little slower — the delays, in milliseconds,
// with the cell each lights.
export function casinoPlan(cells, start, prize, rounds = 3) {
  if (cells < 1) return [];

  const steps = rounds * cells + ((prize - start + cells) % cells || cells);

  return Array.from({ length: steps }, (_, i) => ({
    cell: (start + i + 1) % cells,
    // Fast at first, slowing down to the last steps, as a wheel does.
    delay: Math.round(40 + 360 * (i / steps) ** 3),
  }));
}

// stickersCount is openvk's "stickers_count": "1 стикер", "5 стикеров";
// installsCount is "stickers_installs_count": "2 установки".
export function stickersCount(n) {
  return `${n} ${plural(n, ['стикер', 'стикера', 'стикеров'])}`;
}

export function installsCount(n) {
  return `${n} ${plural(n, ['установка', 'установки', 'установок'])}`;
}

// votesCount is openvk's "points_amount": "1 голос", "3 голоса", "0
// голосов"; giftPrice is what stands under a gift to choose, "Бесплатно"
// for a free one.
export function votesCount(n) {
  return `${n} ${plural(n, ['голос', 'голоса', 'голосов'])}`;
}

export function giftPrice(price) {
  return price > 0 ? votesCount(price) : tr('Бесплатно');
}

// giftCosts are the gifts of a collection of the catalogue with what each
// costs: the administrators price a gift apart from its collection, and a
// server that does not say so prices them all as the collection.
export function giftCosts(collection) {
  return collection.gifts.map((gift, i) => ({ gift, price: collection.prices?.[i] ?? collection.price ?? 0 }));
}

// rubles is a sum in kopecks as it is written in Russian: "7 ₽",
// "7,50 ₽", "1 250 ₽" (no-break spaces between the thousands).
export function rubles(kopecks) {
  const whole = String(Math.trunc(kopecks / 100)).replace(/\B(?=(\d{3})+(?!\d))/g, '\u00a0');
  const rest = kopecks % 100;

  return `${whole}${rest ? `,${String(rest).padStart(2, '0')}` : ''}\u00a0₽`;
}

// voteRate is what a vote is worth: "1 голос = 7 ₽".
export function voteRate(kopecks) {
  return tr('1 голос = {0}', rubles(kopecks));
}

// unusedVotes is openvk's "У Вас N неиспользованных голосов.", in parts:
// the number is bold.
export function unusedVotes(n) {
  return [tr('У Вас '), String(n), ` ${plural(n, ['неиспользованный голос', 'неиспользованных голоса', 'неиспользованных голосов'])}.`];
}

// voucherCode is a serial number as openvk shows it: four groups of six,
// "ABCDEF-GH1234-5678JK-LMNPQR"; voucherParts are the groups.
export function voucherParts(code) {
  return code.match(/.{1,6}/g) ?? [];
}

export function voucherCode(code) {
  return voucherParts(code).join('-');
}

// voucherUsages is "Осталось использований": "∞" for a voucher any number
// of accounts may use; voucherStatus is its "Состояние".
export function voucherUsages(left) {
  return left < 0 ? '∞' : String(left);
}

export function voucherStatus(voucher) {
  return voucher.usagesLeft === 0 ? tr('закончился') : tr('активен');
}

// voucherGave is what an activated voucher gave: "10 голосов и 5%
// рейтинга".
export function voucherGave({ votes, rating }) {
  return [votes > 0 && votesCount(votes), rating > 0 && tr('{0}% рейтинга', rating)].filter(Boolean).join(tr(' и '));
}

// groupedVotes is a number of votes with its thousands apart:
// "1 250 000".
export function groupedVotes(n) {
  return String(Math.trunc(Math.abs(n))).replace(/\B(?=(\d{3})+(?!\d))/g, '\u00a0');
}

// votesAmount is "1 250 000 голосов".
export function votesAmount(n) {
  return `${groupedVotes(n)} ${plural(Math.abs(n), ['голос', 'голоса', 'голосов'])}`;
}

// signedVotes is a line of the statement: "+1 250" or "−300".
export function signedVotes(amount) {
  return `${amount < 0 ? '−' : '+'}${groupedVotes(amount)}`;
}

// voteOperation is what a line of the statement of votes says: its title
// and what the person by it is to the owner — "Получатель", "Отправитель"
// and so on; role is empty when no one is named.
export function voteOperation(op, myId) {
  const out = op.amount < 0;
  const note = op.note ?? '';

  switch (op.kind) {
    case 'transfer':
      return { title: out ? tr('Перевод голосов') : tr('Поступление голосов'), role: out ? tr('Получатель:') : tr('Отправитель:') };
    case 'voucher':
      return { title: tr('Активация ваучера'), role: '' };
    case 'gift': {
      const gift = giftNames[note] ?? tr('Подарок');
      const toSelf = !op.peer || op.peer.id === myId;
      return { title: toSelf ? tr('Покупка подарка «{0}»', gift) : tr('Подарок «{0}»', gift), role: toSelf ? '' : tr('Получатель:') };
    }
    case 'stickers': {
      const pack = stickerPacks[note] ?? note;
      return out
        ? { title: tr('Набор стикеров «{0}»', pack), role: tr('Автор:') }
        : { title: tr('Продажа набора стикеров «{0}»', pack), role: tr('Покупатель:') };
    }
    case 'casino':
      return { title: out ? tr('Казино: прокрутка «Огонька»') : tr('Казино: выигрыш в «Огоньке»'), role: '' };
    case 'aim':
      return { title: out ? tr('Казино: ставка в «Прицеле»') : tr('Казино: выигрыш в «Прицеле»'), role: '' };
    case 'rocket':
      return { title: out ? tr('Казино: ставка в «Ракетке»') : tr('Казино: выигрыш в «Ракетке»'), role: '' };
    case 'stake':
      return { title: out ? tr('Ставка на событие «{0}»', note) : tr('Выплата по событию «{0}»', note), role: '' };
    default:
      return { title: out ? tr('Списание голосов') : tr('Зачисление голосов'), role: '' };
  }
}

// transferNickname is the nickname in the address of a recipient as people
// paste it: "ann", "@ann" or the whole link of the page or of the bank.
export function transferNickname(address) {
  const text = address.trim();
  const to = text.match(/[?&]to=([^&#]+)/);
  if (to) return decodeURIComponent(to[1]);

  return text.replace(/[?#].*$/, '').replace(/\/+$/, '').replace(/^.*\//, '').replace(/^@/, '');
}

// transferURL is the link that opens a transfer to nickname.
export function transferURL(origin, nickname) {
  return `${origin}/votes?act=transfer&to=${encodeURIComponent(nickname)}`;
}

// giftsCount is openvk's "gifts": "3 подарка", "Нет подарков".
export function giftsCount(n) {
  return n === 0 ? tr('Нет подарков') : `${n} ${plural(n, ['подарок', 'подарка', 'подарков'])}`;
}

// Documents ("Мои Документы" openvk). DOCUMENT_FORMATS are the formats
// openvk takes, the end of the file name; a document is up to 10 MB, and
// a post or a message attaches up to 10.
export const DOCUMENT_FORMATS = [
  'jpg', 'jpeg', 'png', 'gif', 'webp', 'psd', 'aep', 'docx', 'doc', 'odt', 'txt', 'md', 'pptx', 'ppt',
  'xls', 'xlsx', 'pdf', 'djvu', 'fb2', 'ps', 'apk', 'zip', '7z', 'mp4', 'avi', 'mp3', 'flac',
];
export const MAX_DOCUMENT_BYTES = 10 << 20;
export const MAX_DOCUMENTS = 10;

// documentFormat is the format of a file name, as the server reads it:
// what follows the last dot, in lower case; '' for none.
export function documentFormat(fileName) {
  const dot = fileName.lastIndexOf('.');

  return dot < 0 ? '' : fileName.slice(dot + 1).toLowerCase();
}

// documentsCount is openvk's "3 документа"; haveDocuments heads "Мои
// Документы", "У Вас 3 документа".
export function documentsCount(n) {
  return `${n} ${plural(n, ['документ', 'документа', 'документов'])}`;
}

export function haveDocuments(n) {
  return tr('У Вас ') + documentsCount(n);
}

// fileSize is openvk's readable_filesize: "512 Б", "1,5 КБ", "10 МБ".
export function fileSize(bytes) {
  const units = [tr('Б'), tr('КБ'), tr('МБ'), tr('ГБ')];
  let n = bytes;
  let unit = 0;

  while (n >= 1024 && unit < units.length - 1) {
    n /= 1024;
    unit++;
  }

  const fraction = String(Math.round(n * 10) / 10);
  const shown = unit === 0 || n >= 10 ? String(Math.round(n)) : lang === 'en' ? fraction : fraction.replace('.', ',');

  return `${shown} ${units[unit]}`;
}

// favesCount is openvk's "N закладок" above what you like.
export function favesCount(n) {
  return n === 0 ? tr('Ни одной закладки') : `${n} ${plural(n, ['закладка', 'закладки', 'закладок'])}`;
}

// photosCount is under an album: openvk writes "N фотографий" whatever N
// ("TODO: Добавить склонения"); we decline it.
export function photosCount(n) {
  return `${n} ${plural(n, ['фотография', 'фотографии', 'фотографий'])}`;
}

// clubRoles name what a member is in a community.
export const clubRoles = { owner: tr('Владелец'), admin: tr('Администратор'), member: tr('Участник') };

export function postsCount(n) {
  return `${n} ${plural(n, ['запись', 'записи', 'записей'])}`;
}

// likedBy is openvk's "Понравилось 3 людям".
export function likedBy(n) {
  return n === 0 ? tr('Никому не понравилось') : tr('Понравилось {0} {1}', n, plural(n, ['человеку', 'людям', 'людям']));
}

// sharedBy heads the list of who shared a post: "Поделились 3 человека".
export function sharedBy(n) {
  if (n === 0) return tr('Никто не поделился');

  return `${plural(n, ['Поделился', 'Поделились', 'Поделились'])} ${n} ${plural(n, ['человек', 'человека', 'человек'])}`;
}

// pollVoters is openvk's line under a poll: how many people voted.
export function pollVoters(n) {
  if (n === 0) return tr('Будьте первым, кто проголосует!');

  return tr('В опросе {0} {1} {2}.', plural(n, ['проголосовал', 'проголосовало', 'проголосовало']), n, plural(n, ['человек', 'человека', 'человек']));
}

// pollMeta is openvk's second line: how the poll is held, its parts
// joined with " • ": "Публичное голосование • много вариантов • нельзя
// переголосовать • до 12 октября в 15:30".
export function pollMeta(poll, now = new Date()) {
  const parts = [poll.anonymous ? tr('Анонимное голосование') : tr('Публичное голосование')];
  if (poll.multiple) parts.push(tr('много вариантов'));
  if (poll.locked) parts.push(tr('нельзя переголосовать'));
  if (poll.endsAt) parts.push(tr('до ') + formatPostDate(poll.endsAt, now));

  return parts.join(' • ');
}

// pollPercent is the share of the voters who chose an option, rounded
// down as openvk does; with several options to choose the shares add up
// to more than 100.
export function pollPercent(votes, voters) {
  return voters > 0 ? Math.min(100, Math.floor((votes / voters) * 100)) : 0;
}

// pollAsksVote: the poll shows its options to choose from, not the
// results: to whoever has not voted, a guest too, while it takes votes.
export function pollAsksVote(poll) {
  return !poll.ended && poll.voted.length === 0;
}

// daysCount is "1 день", "3 дня", "10 дней" of "Голосование истекает
// через".
export function daysCount(n) {
  return `${n} ${plural(n, ['день', 'дня', 'дней'])}`;
}

// wallCount is openvk's counter of a wall: "нет записей", "5 записей".
export function wallCount(n) {
  return n === 0 ? tr('нет записей') : postsCount(n);
}

// archiveYears lays out the years of openvk's "Архив записей", the
// latest first: the first six are links, the rest go under "Ещё". chosen
// is the year the archive shows, 0 for "Все годы"; the year "Действия"
// are about is the chosen one or, without it, the latest.
export function archiveYears(years, chosen = 0, shown = 6) {
  const all = years.includes(chosen) || !chosen ? years : [...years, chosen].sort((a, b) => b - a);

  return { shown: all.slice(0, shown), more: all.slice(shown), action: chosen || all[0] || 0 };
}

// completenessHints are openvk's hints under the gauge of the owner's page:
// what is missing, where to fill it in and what it adds.
export const completenessHints = {
  interests: ['/edit?act=interests', tr('Интересы (+20%)')],
  email: ['/edit?act=contacts', 'Email (+20%)'],
  telegram: ['/edit?act=contacts', 'Telegram (+15%)'],
  status: ['/edit', tr('Статус (+15%)')],
};

// Notification phrases, as on openvk: "<name> оценил(а) вашу запись" and
// so on; the verb agrees with the actor's gender, or is neuter for a
// community: "Сообщество Кошки оставило комментарий".
const notificationVerbs = {
  like: [tr('оценил'), tr('оценила'), tr('оценил(а)'), tr('оценило')],
  comment: [tr('оставил'), tr('оставила'), tr('оставил(а)'), tr('оставило')],
  follow: [tr('подписался'), tr('подписалась'), tr('подписался(-ась)'), tr('подписалось')],
  wall_post: [tr('написал'), tr('написала'), tr('написал(а)'), tr('написало')],
  repost: [tr('поделился'), tr('поделилась'), tr('поделился(-лась)'), tr('поделилось')],
  comment_reply: [tr('ответил'), tr('ответила'), tr('ответил(а)'), tr('ответило')],
  comment_like: [tr('оценил'), tr('оценила'), tr('оценил(а)'), tr('оценило')],
  mention: [tr('упомянул'), tr('упомянула'), tr('упомянул(а)'), tr('упомянуло')],
  gift: [tr('отправил'), tr('отправила'), tr('отправил(а)'), tr('отправило')],
  votes: [tr('передал'), tr('передала'), tr('передал(а)'), tr('передало')],
  invite: [tr('присоединился'), tr('присоединилась'), tr('присоединился(-ась)'), tr('присоединилось')],
  verified: [tr('подтвердил'), tr('подтвердила'), tr('подтвердил(а)'), tr('подтвердило')],
};

// notificationVerb agrees with actor: a person's gender, or a community.
export function notificationVerb(type, actor) {
  const [male, female, unknown, neuter] = notificationVerbs[type];
  if (actor.club) return neuter;
  return { male, female }[actor.gender] ?? unknown;
}

// unreadCountText is the menu counter: nothing at 0, "99+" beyond 99.
export function unreadCountText(n) {
  if (n <= 0) return '';
  return n > 99 ? '99+' : String(n);
}

// snippet shortens a quoted text to about n characters.
export function snippet(text, n) {
  const chars = [...text];
  return chars.length <= n ? text : chars.slice(0, n).join('').trimEnd() + '…';
}

// foundParts splits a text into pieces, those that say the query,
// whatever the case, marked found: a search shows them highlighted.
export function foundParts(text, query) {
  const q = query.trim().toLowerCase();
  const lower = text.toLowerCase();
  // A letter whose lower case is longer would shift the places.
  if (!q || lower.length !== text.length) return [{ text, found: false }];

  const parts = [];
  let from = 0;

  for (let at = lower.indexOf(q); at >= 0; at = lower.indexOf(q, from)) {
    if (at > from) parts.push({ text: text.slice(from, at), found: false });
    parts.push({ text: text.slice(at, at + q.length), found: true });
    from = at + q.length;
  }

  if (from < text.length || parts.length === 0) parts.push({ text: text.slice(from), found: false });

  return parts;
}

// foundSnippet is about n characters of a text from a little before the
// place it first says the query, or its beginning when that is near.
export function foundSnippet(text, query, n) {
  const q = query.trim().toLowerCase();
  const at = text.toLowerCase().indexOf(q);
  if (at < 0 || at + q.length <= n) return snippet(text, n);

  let from = at - Math.floor(n / 4);
  // Not between the halves of an emoji.
  if (text.charCodeAt(from) >= 0xdc00 && text.charCodeAt(from) <= 0xdfff) from++;

  return '…' + snippet(text.slice(from), n);
}

// foundText is the text of a found message that says the query: its own
// or, when a search found the message by what it carries on, that of the
// first forwarded message that does (forwarded tells which).
// Whom a text names is read by name, as in the list of dialogs.
export function foundText(m, query) {
  const q = query.trim().toLowerCase();
  const says = (text) => Boolean(q) && (text ?? '').toLowerCase().includes(q);
  const plain = (text) => plainMentions(text ?? '');
  if (says(plain(m.text))) return { text: plain(m.text), forwarded: false };

  const carried = (m.forwarded ?? []).find((f) => says(plain(f.text)));

  return carried ? { text: plain(carried.text), forwarded: true } : { text: messagePreview(m), forwarded: false };
}

const errorTexts = {
  'network error': tr('Нет связи с сервером. Проверьте подключение.'),
  'invalid email': tr('Похоже, в адресе почты опечатка.'),
  'invalid or expired code': tr('Неверный или устаревший код.'),
  'too many requests, try again later': tr('Слишком часто. Подождите немного и попробуйте снова.'),
  'poll not found': tr('Опрос не найден.'),
  'poll title is empty': tr('Укажите тему опроса.'),
  'poll title is too long': tr('Тема опроса длиннее 128 символов.'),
  'poll option is empty': tr('Вариант ответа не может быть пустым.'),
  'poll option is too long': tr('Вариант ответа длиннее 128 символов.'),
  'a poll has at least one option': tr('Добавьте хотя бы один вариант ответа.'),
  'a poll has at most 10 options': tr('Слишком много вариантов в опросе.'),
  'a poll is open for 1 to 31 days, or for good': tr('Опрос может длиться от 1 до 31 дня.'),
  'invalid choice of poll options': tr('Выберите вариант ответа.'),
  'the poll has ended': tr('Голосование окончено.'),
  'you have voted in the poll already': tr('Вы уже проголосовали в этом опросе.'),
  'you have not voted in the poll': tr('Вы не голосовали в этом опросе.'),
  'the poll does not let a vote be taken back': tr('В этом опросе нельзя отменить голос.'),
  'the poll is anonymous': tr('Невозможно просмотреть список проголосовавших в анонимном голосовании.'),
  'the poll of a post cannot be changed': tr('Опрос записи изменить нельзя.'),
  'too many attempts, request a new code': tr('Слишком много неверных попыток. Запросите новый код.'),
  'name is empty': tr('Укажите имя.'),
  'name is too long': tr('Имя слишком длинное.'),
  'surname is too long': tr('Фамилия слишком длинная.'),
  'nickname is empty': tr('Придумайте никнейм.'),
  'nickname is too long': tr('Никнейм длиннее 32 символов.'),
  'nickname contains invalid characters': tr('Никнейм: латинские буквы, цифры и «_», начинается с буквы, не заканчивается «_».'),
  'nickname is reserved': tr('Этот никнейм зарезервирован, выберите другой.'),
  'nickname is already taken': tr('Этот никнейм уже занят.'),
  'status is too long': tr('Статус длиннее 200 символов.'),
  'text contains invisible characters': tr('Невидимые символы использовать нельзя.'),
  'the label is too long': tr('Метка не может быть длиннее 1000 знаков.'),
  'the label has too many lines': tr('В метке не может быть больше 20 строк.'),
  'birthday is in the future': tr('Дата рождения не может быть в будущем.'),
  'user not found': tr('Страница не найдена.'),
  'session not found': tr('Эта сессия уже завершена.'),
  'post text is empty': tr('Напишите что-нибудь.'),
  'post text is too long': tr('Запись длиннее 60 000 символов.'),
  'post not found': tr('Запись уже удалена.'),
  'the post can no longer be edited': tr('Запись можно редактировать только в течение недели после публикации.'),
  'a post sharing a photo has no photos of its own': tr('К записи с фотографией из альбома нельзя прикрепить другие фотографии.'),
  'a shared photo goes with text alone': tr('Фотографию из альбома можно отправить только с текстом.'),
  'photo not found': tr('Фотография не найдена, прикрепите её снова.'),
  'a post has at most 10 photos': tr('К записи можно прикрепить не больше 10 фотографий.'),
  'the same photo is attached twice': tr('Эта фотография уже прикреплена.'),
  'fill in your profile before posting': tr('Сначала заполните свою страницу.'),
  'this club is closed': tr('Это закрытая группа. Для вступления необходимо подавать заявку.'),
  'join request not found': tr('Заявка не найдена.'),
  'you cannot post on this wall': tr('Хозяин страницы не разрешает вам писать у него на стене.'),
  'you cannot add this user to a chat': tr('Пользователь запретил добавлять себя в беседы.'),
  'a chat needs at least two other people': tr('В беседе должно быть как минимум два других пользователя.'),
  'the chat is full': tr('В беседе не может быть больше 50 человек.'),
  'you are not in this chat': tr('Вы не состоите в этой беседе.'),
  'the user has left the chat': tr('Пользователь сам покинул беседу и может вернуться только сам.'),
  'you were removed from this chat': tr('Вас исключили из беседы: вернуться можно, только если вас снова пригласят.'),
  'you cannot remove yourself, leave the chat instead': tr('Себя нельзя исключить — покиньте чат.'),
  'chat not found': tr('Такой беседы нет.'),
  'member not found': tr('Этого человека нет в беседе.'),
  'you cannot write to this user': tr('Вы не можете отправить сообщение этому пользователю, поскольку он ограничил круг лиц, которые могут присылать ему сообщения.'),
  'you cannot comment on this post': tr('Вы не можете комментировать эту запись.'),
  'topic title is empty': tr('Заголовок не указан.'),
  'topic title is too long': tr('Заголовок длиннее 127 символов.'),
  'topic not found': tr('Тема не найдена или удалена.'),
  'message not found': tr('Сообщение уже удалено.'),
  'you cannot start topics here': tr('Создавать темы в этой группе могут только администраторы.'),
  'you cannot write for this community': tr('Писать от имени сообщества могут только его администраторы.'),
  'the topic is closed': tr('Тема закрыта.'),
  'only a topic of the community can be restricted': tr('Ограничить комментарии можно только в теме от имени сообщества.'),
  'album name is empty': tr('Укажите название альбома.'),
  'album name is too long': tr('Название альбома длиннее 36 символов.'),
  'description is too long': tr('Описание слишком длинное.'),
  'album not found': tr('Альбом не найден или удалён.'),
  'no photos to add': tr('Загрузите хотя бы одну фотографию.'),
  'at most 10 photos are added at once': tr('Вы можете загружать до 10 фотографий за один раз.'),
  'fill in your profile before creating albums': tr('Сначала заполните свою страницу.'),
  'fill in your profile before uploading photos': tr('Сначала заполните свою страницу.'),
  'partner not found': tr('Пользователь с таким никнеймом не найден.'),
  'you cannot name yourself in your relationship': tr('Нельзя указать в семейном положении себя.'),
  'place is too long': tr('Название города длиннее 64 символов.'),
  'invalid telegram username': tr('Неверное имя пользователя Telegram.'),
  'website is not a link': tr('Укажите адрес сайта, например example.com.'),
  'website is too long': tr('Адрес сайта длиннее 256 символов.'),
  'address is too long': tr('Адрес длиннее 255 символов.'),
  'text is too long': tr('Текст длиннее 60 000 символов.'),
  'at most 7 additional fields': tr('Можно добавить не больше 7 полей.'),
  'field name is empty': tr('Укажите название поля.'),
  'field name is too long': tr('Название поля длиннее 50 символов.'),
  'field text is empty': tr('Заполните текст поля.'),
  'you cannot blacklist yourself': tr('Нельзя внести в чёрный список самого себя.'),
  'you cannot ignore yourself': tr('Нельзя игнорировать самого себя.'),
  'too many ignored sources': tr('Превышен лимит игнорируемых источников.'),
  'invalid post id': tr('Запись не найдена.'),
  'comment text is empty': tr('Напишите что-нибудь.'),
  'comment text is too long': tr('Комментарий длиннее 60 000 символов.'),
  'comment not found': tr('Комментарий уже удалён.'),
  'fill in your profile before commenting': tr('Сначала заполните свою страницу.'),
  'notification not found': tr('Уведомление уже удалено.'),
  'message text is empty': tr('Напишите что-нибудь.'),
  'message text is too long': tr('Сообщение слишком длинное.'),
  'message not found': tr('Сообщение не найдено.'),
  'club not found': tr('Группа не найдена.'),
  'club name is empty': tr('Укажите название группы.'),
  'club name is too long': tr('Название группы длиннее 64 символов.'),
  'club description is too long': tr('Описание длиннее 1000 символов.'),
  'club address is too long': tr('Адрес страницы длиннее 32 символов.'),
  'club address contains invalid characters': tr('Адрес страницы может состоять из латинских букв, цифр и _ и начинаться с буквы.'),
  'club address is already taken': tr('Этот адрес уже занят.'),
  'the owner cannot leave the club': tr('Владелец не может выйти из своей группы.'),
  'invalid avatar style': tr('Неизвестный вид аватаров.'),
  'invalid main page': tr('Неизвестная главная страница.'),
  'invalid menu item': tr('Неизвестный пункт меню.'),
  'invalid theme': tr('Неизвестный стиль.'),
  'invalid nsfw setting': tr('Неизвестная настройка NSFW-контента.'),
  'invalid view of posts': tr('Неизвестный вид постов.'),
  'invalid timezone': tr('Неизвестный часовой пояс.'),
  'title is too long': tr('Комментарий слишком длинный: не больше 36 символов.'),
  'the user does not run the club': tr('Этот человек не руководит группой.'),
  'only the admins can add the club to their menu': tr('Добавить группу в левое меню могут только её администраторы.'),
  'at most 10 clubs in the menu': tr('В левом меню может быть не больше 10 групп.'),
  'only the admins can edit the club': tr('Изменять группу могут только её администраторы.'),
  'club website is too long': tr('Адрес сайта длиннее 256 символов.'),
  'club website is not a link': tr('Укажите адрес сайта, например example.com.'),
  'the new owner must be an admin of the club': tr('Владельцем можно назначить только администратора группы.'),
  "this user's clubs are hidden": tr('Пользователь скрыл список своих групп.'),
  "this user's photos are hidden": tr('Пользователь скрыл свои фотографии.'),
  "this user's gifts are hidden": tr('Пользователь скрыл свои подарки.'),
  'you cannot send a gift to this user': tr('Вы не можете отправить подарок этому пользователю.'),
  'fill in your profile before sending gifts': tr('Сначала заполните свою страницу.'),
  'unknown gift': tr('Такого подарка нет.'),
  'gift caption is too long': tr('Сообщение слишком длинное: не больше 250 символов.'),
  'gift not found': tr('Подарок не найден.'),
  'not enough votes': tr('У вас недостаточно голосов.'),
  'a spin must cost from 1 to 10000 votes': tr('Прокрутка должна стоить от 1 до 10 000 голосов.'),
  'the casino must have from 2 to 12 prizes': tr('В казино должно быть от 2 до 12 призов.'),
  'a prize must be from 0 to 1000000 votes': tr('Приз — от 0 до 1 000 000 голосов.'),
  'the weight of a prize must be from 0 to 1000000': tr('Вес приза — от 0 до 1 000 000.'),
  'at least one prize must have a weight': tr('Хотя бы у одного приза вес должен быть больше нуля.'),
  'the bet is out of the limits of the casino': tr('Такую ставку казино не принимает.'),
  'unknown multiplier': tr('Такого множителя нет.'),
  'bets must be from 1 to 100000000 votes, the least not above the most': tr('Ставки — от 1 до 100 000 000 голосов, наименьшая не больше наибольшей.'),
  'bets have from 1 to 10 multipliers': tr('В «Прицеле» должно быть от 1 до 10 множителей.'),
  'a multiplier must be from 1.01 to 1000': tr('Множитель — от ×1,01 до ×1000.'),
  'the chance of a multiplier must be from 0.01 to 99.99 percent': tr('Шанс множителя — от 0,01 до 99,99 %.'),
  'the multipliers repeat': tr('Множители не должны повторяться.'),
  'the question of the event is empty': tr('Напишите вопрос события.'),
  'the question of the event is too long': tr('Вопрос слишком длинный: не больше 200 символов.'),
  'an outcome of the event is empty': tr('Исход не может быть пустым.'),
  'an outcome of the event is too long': tr('Исход слишком длинный: не больше 64 символов.'),
  'an event has from 2 to 10 outcomes': tr('У события должно быть от 2 до 10 исходов.'),
  'the outcomes of the event repeat': tr('Исходы не должны повторяться.'),
  'the stakes must close in the future and within a year': tr('Приём ставок должен закончиться в будущем, но не позже чем через год.'),
  'event not found': tr('Событие не найдено.'),
  'the event takes no more stakes': tr('Ставки на это событие больше не принимаются.'),
  'the event is over already': tr('Событие уже завершено.'),
  'the bet is out of the limits of the rocket': tr('Такую ставку ракета не принимает.'),
  'the rocket cashes by itself from 1.01 to its top': tr('Забрать само можно на множителе от ×1,01 до наибольшего у ракеты.'),
  'rocket not found': tr('Полёт не найден.'),
  'the rocket is over': tr('Этот полёт уже закончился.'),
  'rocket bets must be from 1 to 100000000 votes, the least not above the most': tr('Ставки — от 1 до 100 000 000 голосов, наименьшая не больше наибольшей.'),
  'the return of the rocket must be from 0.01 to 100 percent': tr('Отдача ракеты — от 0,01 до 100 %.'),
  'the top of the rocket must be from 2 to 10000': tr('Наибольший множитель — от ×2 до ×10 000.'),
  'the event is not over yet': tr('Сначала подведите итог события или отмените его.'),
  'unknown outcome': tr('Такого исхода нет.'),
  'a stake must be from 1 to 100000000 votes': tr('Ставка — от 1 до 100 000 000 голосов.'),
  'the odds of an outcome must be from 1.01 to 1000': tr('Коэффициент исхода — от 1,01 до 1000.'),
  'every outcome needs its odds': tr('У каждого исхода должен быть коэффициент.'),
  'the event shares its stakes and has no odds': tr('У этого события нет коэффициентов: ставки на нём делят угадавшие.'),
  'invalid sticker': tr('Такого стикера нет.'),
  'you do not have this sticker': tr('Набор с этим стикером не добавлен.'),
  'a sticker goes alone': tr('Стикер отправляется отдельным сообщением.'),
  'document not found': tr('Документ не найден.'),
  'too many documents': tr('Можно прикрепить не больше 10 документов.'),
  'document is larger than 10 MB': tr('Файл слишком большой: не больше 10 МБ.'),
  'document is empty': tr('Файл пустой.'),
  'this file format is not allowed': tr('Формат файла не разрешён.'),
  'document name is empty': tr('Укажите название документа.'),
  'document name is too long': tr('Название слишком длинное: не больше 128 символов.'),
  'only the owner can change the document': tr('Это не ваш документ.'),
  'the documents of a post cannot be changed': tr('Документы записи нельзя изменить.'),
  'unknown sticker pack': tr('Такого набора стикеров нет.'),
  'sticker pack name is empty': tr('Укажите название набора.'),
  'sticker pack name is too long': tr('Название набора слишком длинное: не больше 64 символов.'),
  'sticker pack description is too long': tr('Описание набора слишком длинное: не больше 500 символов.'),
  'invalid sticker pack price': tr('Стоимость набора — от 0 до 1000 голосов.'),
  'a sticker pack needs a sticker': tr('Добавьте в набор хотя бы один стикер.'),
  'too many stickers in the pack': tr('В наборе может быть не больше 40 стикеров.'),
  'too many sticker packs': tr('Можно создать не больше 20 наборов стикеров.'),
  'only the author can change a sticker pack': tr('Это не ваш набор стикеров.'),
  'the sticker pack is taken by others': tr('Набор уже установили другие: удалить его или его стикеры нельзя.'),
  'fill in your profile before making stickers': tr('Сначала заполните свою страницу.'),
  'voucher is not valid': tr('Ваучер недействительный. Возможно, вы ввели неверный серийный номер, уже использовали данный ваучер или же он просто истёк.'),
  'a ban needs a reason': tr('Укажите причину блокировки.'),
  'the reason of a ban is too long': tr('Причина блокировки слишком длинная.'),
  'a ban must end in the future': tr('Блокировка должна кончаться в будущем.'),
  'invalid until': tr('Неверная дата окончания блокировки.'),
  'you cannot ban yourself': tr('Нельзя заблокировать самого себя.'),
  'an administrator cannot be banned': tr('Администратора сайта заблокировать нельзя.'),
  'you cannot transfer votes to yourself': tr('Нельзя передать голоса самому себе.'),
  'you cannot transfer votes to this user': tr('Вы не можете передать голоса этому пользователю.'),
  'the number of votes must be between 1 and 100000000': tr('Количество голосов — от 1 до 100 000 000.'),
  'the message of a transfer is too long': tr('Сообщение к переводу — не больше 140 символов.'),
  'the message of a transfer is one line': tr('Сообщение к переводу — одна строка.'),
  'voucher serial number must be 24 letters and digits': tr('Серийный номер — 24 латинские буквы и цифры.'),
  'voucher must give votes or rating': tr('Ваучер должен давать голоса или рейтинг.'),
  'voucher gives up to 100000000 votes and up to 1000 of rating': tr('Ваучер даёт не больше 100 000 000 голосов и 1000 рейтинга.'),
  'voucher usages must be -1 or between 1 and 1000000': tr('Количество использований — от 1 до 1 000 000 или -1.'),
  'voucher with this serial number already exists': tr('Ваучер с таким серийным номером уже есть.'),
  'only the sender can edit the message': tr('Редактировать можно только свои сообщения.'),
  'the message can no longer be edited': tr('Сообщение можно редактировать только в течение суток после отправки.'),
  'the title was changed': tr('Комментарий уже изменил другой руководитель. Обновите страницу и попробуйте снова.'),
  'the message was changed': tr('Сообщение изменили, пока вы его редактировали. Отмените правку и начните её заново.'),
  'messages are end-to-end encrypted': tr('Сообщения диалогов шифруются: обновите страницу.'),
  'messages of dialogs are end-to-end encrypted': tr('Сообщения диалогов зашифрованы: искать в них можно только на странице диалога.'),
  'push notifications are off': tr('Сайт пока не отправляет уведомления.'),
  'invalid push endpoint': tr('Браузер не смог подписаться на уведомления.'),
  'invalid push keys': tr('Браузер не смог подписаться на уведомления.'),
  'the encryption keys changed': tr('Ключ шифрования собеседника изменился. Обновите страницу и отправьте ещё раз.'),
  'encrypted messages cannot be forwarded into a chat': tr('Сообщения диалогов зашифрованы, их нельзя переслать в беседу.'),
  'messages of chats are not encrypted': tr('Сообщения бесед не шифруются: обновите страницу.'),
  'invalid encryption key': tr('Неверный ключ шифрования.'),
  'invalid encrypted message': tr('Не удалось зашифровать сообщение.'),
  'encrypted message is too long': tr('Сообщение слишком длинное.'),
  'forwarded message not found': tr('Сообщение не удалось переслать: его уже нет.'),
  'replied message not found': tr('Сообщение, на которое вы отвечаете, уже недоступно.'),
  'invalid reaction': tr('Такой реакции нет.'),
  'a message forwards at most 20 messages': tr('За один раз можно переслать не больше 20 сообщений.'),
  'search query is empty': tr('Введите, что искать.'),
  'the comments are closed': tr('Комментарии к этой записи закрыты.'),
  'search query is too long': tr('Запрос длиннее 100 символов.'),
  'this album is kept by the site': tr('Этот альбом сайт ведёт сам: его нельзя изменить.'),
  'a photo moves between the albums of one owner': tr('Фотографию можно перенести только в другой альбом той же страницы.'),
  "this user's follows are hidden": tr('Пользователь скрыл список своих подписок.'),
  'this page is hidden': tr('Настройки приватности этого пользователя не разрешают вам смотреть на его страницу.'),
  'you cannot suggest posts here': tr('В эту группу нельзя предлагать записи.'),
  'suggestion not found': tr('Запись уже опубликована или отклонена.'),
  'postponed post not found': tr('Запись уже опубликована или удалена.'),
  'invalid publish time': tr('Укажите дату и время публикации.'),
  'a postponed post is published within a year from now': tr('Время публикации должно быть в будущем, но не позже чем через год.'),
  'a postponed post says words and shows photos only': tr('К отложенной записи можно прикрепить только фотографии.'),
  'too many postponed posts': tr('Отложено слишком много записей: не больше 100 на стену.'),
  'too many stories': tr('Сразу можно показывать не больше 30 историй.'),
  'ad link must be an http or https link': tr('Ссылка объявления должна вести на сайт: http:// или https://.'),
  'ad link is too long': tr('Ссылка объявления слишком длинная.'),
  'ad title is empty': tr('Укажите заголовок объявления.'),
  'ad title is too long': tr('Заголовок объявления — не длиннее 100 символов.'),
  'ad text is too long': tr('Текст объявления — не длиннее 500 символов.'),
  'ad not found': tr('Такого объявления нет.'),
  'too many ads': tr('Объявлений слишком много: не больше 100. Удалите ненужные.'),
  'story not found': tr('Этой истории уже нет.'),
  'story comment not found': tr('Этого комментария уже нет.'),
  'story comment is too long': tr('Комментарий к истории — не больше 1000 символов.'),
  'you cannot comment on this story': tr('Вы не можете комментировать эту историю.'),
  'story text is too long': tr('Подпись истории — не больше 200 символов.'),
  'a story shows a photo': tr('Выберите фотографию для истории.'),
  'fill in your profile before creating a club': tr('Сначала заполните свою страницу.'),
  'fill in your profile before joining a club': tr('Сначала заполните свою страницу.'),
  'the message can no longer be deleted': tr('Сообщение можно удалить только в течение суток после отправки.'),
  'you cannot write to yourself': tr('Нельзя написать самому себе.'),
  'fill in your profile before writing messages': tr('Сначала заполните свою страницу.'),
  'permission denied': tr('Недостаточно прав.'),
  'this page is deleted': tr('Страница пользователя заблокирована или удалена.'),
  'this page is not deleted': tr('Ваша страница не удалена.'),
  'this page can no longer be restored': tr('Срок восстановления страницы истёк.'),
  'reason is too long': tr('Сообщение длиннее 1000 символов.'),
  'you cannot follow yourself': tr('Нельзя подписаться на себя.'),
  'image is larger than 8 MB': tr('Фотография больше 8 МБ.'),
  'not a JPEG, PNG, GIF or WebP image': tr('Это не фотография: подойдут JPEG, PNG, GIF и WebP.'),
  'image dimensions are too large': tr('Слишком большое разрешение фотографии.'),
  'image is smaller than 50x50': tr('Фотография меньше 50×50 точек.'),
  'search query is empty': tr('Введите имя, фамилию или никнейм.'),
  'search query is too long': tr('Слишком длинный запрос.'),
  'fill in your profile before following': tr('Сначала заполните свою страницу.'),
  'service temporarily unavailable': tr('Сервис временно недоступен. Попробуйте чуть позже.'),
  'missing access token': tr('Сессия истекла, войдите снова.'),
  'invalid access token': tr('Сессия истекла, войдите снова.'),
  'invalid refresh token': tr('Сессия истекла, войдите снова.'),
};

// keyStateText says why this browser cannot open and seal messages of
// dialogs (e2ee.js): the key of the page is in another browser
// ('missing'), or this one has an older key ('stale').
export function keyStateText(state) {
  return state === 'stale'
    ? tr('На этом устройстве старый ключ шифрования сообщений: новый создан на другом устройстве.')
    : tr('На этом устройстве нет ключа шифрования сообщений: он на другом устройстве.');
}

// pushStateTexts say what the push notifications of this browser are
// (push.js pushState).
export const pushStateTexts = {
  on: tr('Уведомления включены.'),
  off: tr('Уведомления выключены.'),
  denied: tr('Уведомления запрещены в настройках браузера. Чтобы включить их, разрешите их этому сайту там.'),
  'off-server': tr('Сайт пока не отправляет уведомления.'),
  unsupported: tr('Этот браузер не получает уведомления.'),
  install: tr('На iPhone и iPad уведомления приходят, только когда сайт добавлен на экран «Домой»: нажмите «Поделиться», затем «На экран «Домой»» и откройте сайт оттуда.'),
};

// pushOfferText is what a phone is asked once over its pages (layout.js
// offerPush).
export const pushOfferText = tr('Получать на этом телефоне уведомления о новых сообщениях и ответах, даже когда сайт закрыт?');

export function errorText(e) {
  return errorTexts[e?.message] ?? tr('Что-то пошло не так. Попробуйте позже.');
}

// mentionPattern finds the two ways a text names a person, as the gateway
// does (model.MentionedNicknames): "[nickname|Имя]", which "Ответить"
// writes, and "@nickname" at the start or after a space.
const mentionPattern = /\[([A-Za-z][A-Za-z0-9_]*)\|([^\[\]|\n]{1,100})\]|(^|\s)@([A-Za-z][A-Za-z0-9_]*)/g;

const MAX_NICKNAME = 32;

// mentionParts cuts text into what it shows: strings and, for the people it
// names, { nickname, text } to draw as links to their pages. Whether anyone
// has the nickname only the server knows (see nicknames.js).
export function mentionParts(text) {
  const parts = [];
  let from = 0;

  const push = (s) => {
    if (!s) return;
    if (typeof parts.at(-1) === 'string') parts[parts.length - 1] += s;
    else parts.push(s);
  };

  for (const m of text.matchAll(mentionPattern)) {
    const named = m[1] !== undefined;
    // A nickname does not end with "_": "@ann_" names ann.
    const nickname = named ? m[1] : m[4].replace(/_+$/, '');
    if (nickname.length > MAX_NICKNAME || nickname.endsWith('_')) continue;

    const start = m.index + (named ? 0 : m[3].length);
    const length = named ? m[0].length : 1 + nickname.length;

    push(text.slice(from, start));
    parts.push({ nickname, text: named ? m[2] : '@' + nickname });
    from = start + length;
  }

  push(text.slice(from));

  return parts;
}

// hashtagPattern finds the tags a text says ("#поход"), as post does
// (post.Text.Hashtags): "#" at the start or after a space, then letters,
// digits and "_", up to MAX_HASHTAG of them.
const hashtagPattern = /(^|\s)#([\p{L}\p{Nd}\p{Mn}_]+)/gu;

const MAX_HASHTAG = 64;

// hashtagPath is the search of the posts that say a tag, where openvk
// links it.
export function hashtagPath(tag) {
  return '/search?section=posts&q=' + encodeURIComponent('#' + tag);
}

// textParts is mentionParts with the web addresses and the tags too: the
// strings say { url, text } of each address and { hashtag, text } of each
// tag, without "#". Inside an address "#" and "@" are its own.
export function textParts(text) {
  return mentionParts(text).flatMap((part, i) => {
    if (typeof part !== 'string') return [part];

    return linkParts(part).flatMap((p, j) => (typeof p === 'string' ? hashtagParts(p, i === 0 && j === 0) : [p]));
  });
}

// linkPattern finds a web address as the gateway does (model.FirstLink):
// http or https, up to a space or a character no address has.
const linkPattern = /\bhttps?:\/\/[^\s<>"«»[\]]+/g;

// MAX_LINK_SHOWN: a longer address is shown cut, as VK does.
const MAX_LINK_SHOWN = 60;

// linkParts cuts s into strings and the addresses it says, { url, text }:
// text is the address as shown, cut when long.
export function linkParts(s) {
  const parts = [];
  let from = 0;

  for (const m of s.matchAll(linkPattern)) {
    const url = trimLink(m[0]);
    if (!url) continue;

    if (m.index > from) parts.push(s.slice(from, m.index));
    parts.push({ url, text: shownLink(url) });
    from = m.index + url.length;
  }

  if (from < s.length) parts.push(s.slice(from));

  return parts;
}

// trimLink drops what ends a sentence rather than an address: ".", ",",
// "!" and the like, and ")" that closes no "(" of the address.
function trimLink(link) {
  for (;;) {
    let trimmed = link.replace(/[.,;:!?'"’”…]+$/u, '');
    const opened = trimmed.split('(').length;
    if (trimmed.endsWith(')') && opened < trimmed.split(')').length) trimmed = trimmed.slice(0, -1);

    if (trimmed === link) break;
    link = trimmed;
  }

  return link.endsWith('://') ? '' : link;
}

// shownLink is an address without its scheme, cut when long.
export function shownLink(url) {
  const shown = url.replace(/^https?:\/\//, '').replace(/^www\./, '');
  const chars = [...shown];

  return chars.length > MAX_LINK_SHOWN ? chars.slice(0, MAX_LINK_SHOWN - 1).join('') + '…' : shown;
}

// hashtagParts cuts a string of a text into strings and tags; atStart: the
// string starts the text, else it follows a mention, which is no space.
function hashtagParts(s, atStart) {
  const parts = [];
  let from = 0;

  for (const m of s.matchAll(hashtagPattern)) {
    if ([...m[2]].length > MAX_HASHTAG || (m.index === 0 && !m[1] && !atStart)) continue;

    const start = m.index + m[1].length;
    if (start > from) parts.push(s.slice(from, start));
    parts.push({ hashtag: m[2], text: '#' + m[2] });
    from = start + 1 + m[2].length;
  }

  if (from < s.length) parts.push(s.slice(from));

  return parts;
}

// plainMentions is text as a quote shows it, without links: "[ann|Анна]"
// reads "Анна".
export function plainMentions(text) {
  return mentionParts(text).map((p) => (typeof p === 'string' ? p : p.text)).join('');
}

// replyMention starts an answer to author, openvk's "[id1|Анна], ": a
// link to the person, who hears about the answer. A community is named in
// plain words.
export function replyMention(author) {
  return author.club || !author.nickname ? author.name + ', ' : `[${author.nickname}|${author.name}], `;
}

// isLongText is openvk's "bigText", shown cut short until "Показать
// больше": more than 520 characters or 10 lines.
export function isLongText(text) {
  return text.length > 520 || text.split('\n').length > 10;
}

// forwardedCount names the messages a message carries on: "2 пересланных
// сообщения".
export function forwardedCount(n) {
  return n === 1 ? tr('Пересланное сообщение') : `${n} ${plural(n, ['пересланное сообщение', 'пересланных сообщения', 'пересланных сообщений'])}`;
}

// forwardTree nests the messages a message carries on: the list names a
// message before those it carried on itself, each one level deeper
// (depth). The trees are { message, carried }, oldest first.
// photoChanges is what an edit does to the photos of a message: the
// IDs it adds to those the message showed when the edit began and the
// IDs it takes away.
export function photoChanges(before, ids) {
  const had = (before ?? []).map((p) => p.id);

  return { add: ids.filter((id) => !had.includes(id)), remove: had.filter((id) => !ids.includes(id)) };
}

export function forwardTree(forwarded) {
  const top = [];
  // path[d] is where a message of depth d goes.
  const path = [top];

  for (const f of forwarded) {
    const depth = Math.min(Math.max(f.depth ?? 0, 0), path.length - 1);
    const tree = { message: f, carried: [] };
    path[depth].push(tree);
    path.length = depth + 1;
    path.push(tree.carried);
  }

  return top;
}

// messagePreview is a message in one line, for the list of dialogs: what
// it says or, without words, what it shows, as on VK. Whom the text
// names is read by name: "[ann|Анна]" is "Анна".
export function messagePreview(m) {
  if (m.deleted) return tr('Сообщение удалено');
  if (m.sticker) return tr('Стикер');
  if (m.unreadable) return tr('Зашифрованное сообщение');
  if (m.text) return plainMentions(m.text);
  if (m.photos?.length || m.albumPhoto) return tr('Фотография');
  if (m.post) return tr('Запись');
  if (m.documentCount || m.documents?.length) return tr('Документ');
  if (m.forwardedCount) return forwardedCount(m.forwardedCount);

  return '';
}

// Answers ("Ответить", as on VK): over its words an answer shows who
// wrote the message it answers and, in one line, its beginning. A dialog's
// answer carries those words sealed itself (e2ee.js): the server cannot
// quote them.
export const QUOTE_LENGTH = 200;

export function quoteSnippet(text) {
  const line = text.replace(/\s+/g, ' ').trim();
  const chars = [...line];
  return chars.length <= QUOTE_LENGTH ? line : chars.slice(0, QUOTE_LENGTH).join('').trimEnd() + '…';
}

// quoteText is what the line says of q, the message answered; text is
// its words as the page knows them.
export function quoteText(q, text = q.text) {
  if (!q.author) return tr('Сообщение недоступно');

  return messagePreview({
    deleted: q.deleted, sticker: q.sticker, text: text ? quoteSnippet(text) : '', photos: q.photoCount ? [q.id] : [],
    albumPhoto: q.albumPhoto, post: q.post, documentCount: q.documentCount, forwardedCount: q.forwardedCount,
  }) || tr('Сообщение');
}

// Reactions to messages, as on VK: one a person, of these.
export const REACTIONS = [
  { name: 'heart', emoji: '❤️', title: tr('Сердечко') },
  { name: 'like', emoji: '👍', title: tr('Нравится') },
  { name: 'laugh', emoji: '😂', title: tr('Смешно') },
  { name: 'wow', emoji: '😮', title: tr('Удивительно') },
  { name: 'sad', emoji: '😢', title: tr('Грустно') },
  { name: 'fire', emoji: '🔥', title: tr('Огонь') },
];

// reactionCounts are the reactions put on a message, in the order of
// REACTIONS: how many of each and whether one is userID's.
export function reactionCounts(reactions = [], userID = '') {
  return REACTIONS.map((r) => ({
    ...r,
    count: reactions.filter((x) => x.reaction === r.name).length,
    mine: reactions.some((x) => x.reaction === r.name && x.userId === userID),
  })).filter((r) => r.count > 0);
}

// reacted is reactions with userID's changed to reaction ('' takes it
// away), as the server will have them.
export function reacted(reactions = [], userID, reaction) {
  const others = reactions.filter((x) => x.userId !== userID);
  return reaction ? [...others, { userId: userID, reaction }] : others;
}

// Chats of several people ("беседы"). A chat without a title of its own
// is "Беседа"; a new one left untitled is named by who is in it, as VK
// names them: "Анна, Пётр, Иван", at most 64 characters.
export const CHAT_TITLE_MAX = 64;

export function chatTitle(chat) {
  return chat.title || tr('Беседа');
}

export function defaultChatTitle(people) {
  let title = '';

  for (const p of people) {
    const next = title ? `${title}, ${p.name}` : p.name;
    if ([...next].length > CHAT_TITLE_MAX) break;
    title = next;
  }

  return title;
}

// chatMembers is under the title of a chat: "3 участника".
export function chatMembers(n) {
  return `${n} ${plural(n, ['участник', 'участника', 'участников'])}`;
}

// accusativeName is "Петра Петрова", "Анну Иванову": whom someone added
// to a chat or removed from it.
export function accusativeName(person) {
  return declinedName(person, 'acc');
}

// chatActionParts is what a message of a chat tells of instead of saying
// something, after the name of who did it: "создала беседу «Поход»",
// "пригласил Петра Петрова", "исключил Петра Петрова из беседы" — as what
// stands before the name of whom it is about, that name, and what follows.
export function chatActionParts(m) {
  const g = m.author?.gender;
  const whom = m.target ? accusativeName(m.target) : '';

  switch (m.action) {
    case 'create':
      return [tr('{0} беседу', byGender(g, tr('создал'), tr('создала'))) + (m.text ? ` «${m.text}»` : ''), '', ''];
    case 'title':
      return [m.text
        ? tr('{0} название беседы на «{1}»', byGender(g, tr('изменил'), tr('изменила')), m.text)
        : tr('{0} название беседы', byGender(g, tr('убрал'), tr('убрала'))), '', ''];
    case 'invite':
      return [`${byGender(g, tr('пригласил'), tr('пригласила'))} `, whom, ''];
    case 'kick':
      return [`${byGender(g, tr('исключил'), tr('исключила'))} `, whom, tr(' из беседы')];
    case 'leave':
      return [tr('{0} беседу', byGender(g, tr('покинул'), tr('покинула'))), '', ''];
    case 'return':
      return [tr('{0} в беседу', byGender(g, tr('вернулся'), tr('вернулась'))), '', ''];
    case 'pin':
      return [tr('{0} сообщение', byGender(g, tr('закрепил'), tr('закрепила'))), '', ''];
    case 'unpin':
      return [tr('{0} сообщение', byGender(g, tr('открепил'), tr('открепила'))), '', ''];
    case 'photo':
      return [tr('{0} фотографию беседы', byGender(g, tr('обновил'), tr('обновила'))), '', ''];
    case 'photo_remove':
      return [tr('{0} фотографию беседы', byGender(g, tr('удалил'), tr('удалила'))), '', ''];
    default:
      return ['', '', ''];
  }
}

export function chatAction(m) {
  return chatActionParts(m).join('');
}

// chatPreview is the last message of a chat in one line, for the list of
// dialogs: who and what, "Пётр: иду" or "Анна создала беседу «Поход»".
export function chatPreview(m) {
  if (!m) return '';
  if (m.action) return `${m.author.name} ${chatAction(m)}`;

  const said = messagePreview(m);

  return m.out ? said : `${m.author.name}: ${said}`;
}

// A conversation a message is sent on to, a dialog or a chat, as the
// value of a choice: "user:<nickname>" and "chat:<id>" (a nickname has no
// colon). conversationOf reads it back, conversationPath is its page.
export function conversationValue(d) {
  return d.chat ? 'chat:' + d.chat.id : 'user:' + d.peer.nickname;
}

export function conversationOf(value) {
  const at = value.indexOf(':');

  return { chat: value.slice(0, at) === 'chat', ref: value.slice(at + 1) };
}

export function conversationPath(value) {
  const { chat, ref } = conversationOf(value);

  return (chat ? '/im/chat/' : '/im/') + encodeURIComponent(ref);
}

// chatStateText is why someone who is not in a chat anymore cannot write
// there.
export function chatStateText(state) {
  return state === 'kicked' ? tr('Вас исключили из беседы.') : tr('Вы покинули беседу.');
}

// selectedCount is over a dialog while messages are selected: "Выбрано 2
// сообщения".
export function selectedCount(n) {
  return tr('Выбрано {0}', messagesCount(n));
}

// messagesCount is "2 сообщения".
export function messagesCount(n) {
  return `${n} ${plural(n, ['сообщение', 'сообщения', 'сообщений'])}`;
}

// "Настройки ленты" of openvk: how many posts a page of the news holds
// (its choices) and whether what people wrote on the walls of others is
// there. They are kept by the browser, as openvk keeps them in a cookie.
export const feedPageSizes = [1, 5, 10, 20, 30, 40, 50];

// Ten posts a page, and the posts on the walls of others are there unless
// left out (openvk leaves them out by default; the owner of the site
// wants them).
export const defaultFeedSettings = Object.freeze({ posts: 10, alien: true });

// feedSettings reads what the browser kept; anything else is the default.
export function feedSettings(raw) {
  let kept = null;

  try {
    kept = JSON.parse(raw ?? 'null');
  } catch {
    kept = null;
  }

  return {
    posts: feedPageSizes.includes(kept?.posts) ? kept.posts : defaultFeedSettings.posts,
    alien: typeof kept?.alien === 'boolean' ? kept.alien : defaultFeedSettings.alien,
  };
}

// ignoreLabels are the link of a page: openvk's "Игнорировать
// пользователя" and "Игнорировать группу".
export function ignoreLabel(kind, ignored) {
  const whom = { user: tr('пользователя'), club: tr('группу') }[kind];

  return (ignored ? tr('Не игнорировать ') : tr('Игнорировать ')) + whom;
}

// typingText is VK's "печатает..." under a dialog or a chat: who of users
// (by first name) is writing now; '' for nobody.
export function typingText(users) {
  const names = users.map((u) => u.name || u.nickname);

  switch (names.length) {
    case 0:
      return '';
    case 1:
      return tr('{0} печатает...', names[0]);
    case 2:
      return tr('{0} и {1} печатают...', names[0], names[1]);
    default:
      return tr('Несколько человек печатают...');
  }
}

// A nickname as the user service takes it (user/internal/user/nickname.go).
const nicknameShape = /^[A-Za-z](?:[A-Za-z0-9_]*[A-Za-z0-9])?$/;

// inviteNickname is who invites by the link of the page, openvk's
// "/reg?ref=": "?invite=ann" on the main page; '' for none or a word that
// is no nickname.
export function inviteNickname(search) {
  const nickname = new URLSearchParams(search).get('invite') ?? '';
  return nickname.length <= 32 && nicknameShape.test(nickname) ? nickname : '';
}

// inviteURL is the link of "Пригласить" that invites in the name of
// nickname.
export function inviteURL(origin, nickname) {
  return `${origin}/?invite=${encodeURIComponent(nickname)}`;
}

// newYearGreeting is the greeting under the fir tree of the New Year
// decoration: the coming year until it comes — and in the months before,
// when the administrators turn the decoration on early — then the year
// that came.
export function newYearGreeting(date) {
  return date.getMonth() >= 6 ? tr('С наступающим Новым годом!') : tr('С Новым годом!');
}

// activeSection is the section of the bar at the bottom of a phone a path
// is in (layout.js): the page of whoever signed in, ownPath, the news, the
// messages — dialogs and chats too — and the notifications; '' for others.
export function activeSection(path, ownPath) {
  if (path === '/feed' || path.startsWith('/feed/')) return 'feed';
  if (path === '/im' || path.startsWith('/im/')) return 'messages';
  if (path === '/notifications') return 'notifications';
  if (ownPath && decodeURIComponent(path) === ownPath) return 'page';

  return '';
}
