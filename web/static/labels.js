// Russian texts for API values and errors.

// genders are "Пол" of the profile form. The site has no pronouns: the
// gender is asked only because Russian words agree with it. openvk's
// "они/их" (neutral) is no longer offered, and a page that chose it is
// spoken of as one whose gender is not set.
export const genders = [
  ['', 'не указан'],
  ['male', 'мужской'],
  ['female', 'женский'],
];

// Relationship labels agree with gender, as in Russian they must; '' is
// for a gender not set.
const relationshipLabels = {
  single: { male: 'не женат', female: 'не замужем', '': 'не женат / не замужем' },
  dating: { male: 'встречается', female: 'встречается', '': 'встречается' },
  engaged: { male: 'помолвлен', female: 'помолвлена', '': 'помолвлен(а)' },
  married: { male: 'женат', female: 'замужем', '': 'в браке' },
  civil_union: { male: 'в гражданском браке', female: 'в гражданском браке', '': 'в гражданском браке' },
  in_love: { male: 'влюблён', female: 'влюблена', '': 'влюблён(а)' },
  complicated: { male: 'всё сложно', female: 'всё сложно', '': 'всё сложно' },
  looking: { male: 'в активном поиске', female: 'в активном поиске', '': 'в активном поиске' },
};

export function relationships(gender) {
  return [['', 'не выбрано'], ...Object.entries(relationshipLabels).map(([value, byGender]) => [value, byGender[gender] ?? byGender['']])];
}

// partnerPrefixes are openvk's "relationship_N_prefix": the word before
// whom a relationship names, "Женат на …", "Замужем за …".
const partnerPrefixes = {
  dating: 'с', engaged: 'с', civil_union: 'с', complicated: 'с', in_love: 'в',
  married: { male: 'на', female: 'за', '': 'с' },
};

// takesPartner: the relationship names someone, as in the user service.
export function takesPartner(relationship) {
  return relationship in partnerPrefixes;
}

export function partnerPrefix(relationship, gender) {
  const prefix = partnerPrefixes[relationship];
  return typeof prefix === 'string' ? prefix : prefix[gender] ?? prefix[''];
}

// The case each of the prefixes asks of the name after it.
const prefixCases = { с: 'ins', за: 'ins', на: 'pre', в: 'acc' };

// partnerName is the partner after the prefix: "замужем за Петром
// Петровым", "женат на Анне Ивановой", "влюблена в Петра Петрова". openvk
// leaves the name as it is.
export function partnerName(relationship, gender, partner) {
  return declinedName(partner, prefixCases[partnerPrefix(relationship, gender)]);
}

export const worldviews = [
  ['', 'не выбрано'],
  ['orthodox', 'православие'],
  ['catholicism', 'католицизм'],
  ['protestantism', 'протестантизм'],
  ['islam', 'ислам'],
  ['judaism', 'иудаизм'],
  ['buddhism', 'буддизм'],
  ['confucianism', 'конфуцианство'],
  ['secular_humanism', 'светский гуманизм'],
];

export const politicalViews = [
  ['', 'не выбраны'],
  ['indifferent', 'индифферентные'],
  ['communist', 'коммунистические'],
  ['socialist', 'социалистические'],
  ['liberal', 'либеральные'],
  ['conservative', 'консервативные'],
  ['monarchist', 'монархические'],
  ['ultraconservative', 'ультраконсервативные'],
  ['libertarian', 'либертарианские'],
  ['moderate_islamism', 'умеренный исламизм'],
  ['radical_islamism', 'радикальный исламизм'],
];

// wallWrites is "Кто может писать у меня на стене" of openvk. There are no
// friends here: openvk's "Друзья" are the people you follow.
export const wallWrites = [
  ['everyone', 'все желающие'],
  ['following', 'те, на кого я подписан(а)'],
  ['only_me', 'только я'],
];

// messagesWrites is "Кто может писать мне сообщения" of openvk, with its
// "Друзья" as the people you follow; whoever you wrote to may answer.
// chatsInvites are "Кто может добавлять меня в чаты" of openvk.
export const chatsInvites = [
  ['everyone', 'все желающие'],
  ['following', 'те, на кого я подписан(а)'],
  ['nobody', 'никто'],
];

// profileTypes is "Тип профиля" of openvk.
export const profileTypes = [
  ['open', 'открытый'],
  ['closed', 'закрытый'],
];

// clubTypes is "Тип группы" of openvk; clubTypeHints is what the page of
// a group says under it.
export const clubTypes = [
  ['open', 'открытая'],
  ['closed', 'закрытая'],
];

export const clubTypeHints = {
  open: 'Это открытая группа. В неё может вступить любой желающий.',
  closed: 'Это закрытая группа. Для вступления необходимо подавать заявку.',
};

// requestsCount is the head of "Заявки" of a closed group, as on openvk.
export function requestsCount(n) {
  if (n <= 0) return 'Не найдено ни одной заявки...';
  return `${plural(n, ['Найдена', 'Найдено', 'Найдено'])} ${n} ${plural(n, ['заявка', 'заявки', 'заявок'])}`;
}

// requestsLink is the link to them on the page of the group.
export function requestsLink(n) {
  return `${n} ${plural(n, ['заявка', 'заявки', 'заявок'])} на вступление`;
}

export const messagesWrites = [
  ['everyone', 'все желающие'],
  ['following', 'те, на кого я подписан(а)'],
  ['nobody', 'никто'],
];

// audiences are who sees a part of a page ("Кому видно мои группы",
// "… мои фотографии", "… моих друзей", "… основную информацию моей
// страницы" of openvk), with its "Друзьям" as the people you follow.
export const audiences = [
  ['everyone', 'всем желающим'],
  ['users', 'пользователям Freenet'],
  ['following', 'тем, на кого я подписан(а)'],
  ['only_me', 'только мне'],
];

// pageReads is "Кому в интернете видно мою страницу" of openvk.
export const pageReads = audiences.slice(0, 2);

// onlineReads is "Кто видит мою неактивность" of openvk, in its words.
export const onlineReads = [
  ['everyone', 'все'],
  ['users', 'пользователи Freenet'],
  ['following', 'те, на кого я подписан(а)'],
  ['only_me', 'никто'],
];

// adminsDisplays is "Список админов" of a community's settings in openvk.
export const adminsDisplays = [
  ['creator', 'отображать только создателя группы'],
  ['all', 'отображать всех администраторов'],
  ['none', 'ничего не отображать'],
];

export const months = [
  'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
  'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря',
];

// label is the text of a set value; "" for a value that is not set, so the
// profile shows no row instead of the form's "не выбрано".
export function label(options, value) {
  if (!value) return '';
  return options.find(([v]) => v === value)?.[1] ?? '';
}

export function plural(n, [one, few, many]) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

// formatBirthday turns "1990-05-17" into "17 мая 1990 г., 35 лет".
// A birthday shown without its year comes as "--05-17": "17 мая", as
// openvk's "Показывать только день и месяц".
export function formatBirthday(iso) {
  if (iso.startsWith('--')) {
    const [month, day] = iso.slice(2).split('-').map(Number);
    return `${day} ${months[month - 1]}`;
  }

  const [year, month, day] = iso.split('-').map(Number);
  const today = new Date();

  let age = today.getFullYear() - year;
  if (today.getMonth() + 1 < month || (today.getMonth() + 1 === month && today.getDate() < day)) age--;

  return `${day} ${months[month - 1]} ${year} г., ${age} ${plural(age, ['год', 'года', 'лет'])}`;
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

  if (daysAgo === 0) return `сегодня в ${time}`;
  if (daysAgo === 1) return `вчера в ${time}`;

  const day = `${date.getUTCDate()} ${months[date.getUTCMonth()]}`;

  return date.getUTCFullYear() === today.getUTCFullYear() ? `${day} в ${time}` : `${day} ${date.getUTCFullYear()} в ${time}`;
}

// formatDateTime turns an ISO time into openvk's "7 мая 2027 в 12:00".
export function formatDateTime(iso) {
  const date = onClock(new Date(iso));
  const time = `${date.getUTCHours()}:${two(date.getUTCMinutes())}`;

  return `${date.getUTCDate()} ${months[date.getUTCMonth()]} ${date.getUTCFullYear()} в ${time}`;
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
  ['square', 'квадратные (по умолчанию)'],
  ['round', 'круглые'],
];

// themes, ratings, nsfwContents and postViews are the rest of openvk's
// "Интерфейс"; the themes are ours.
export const themes = [
  ['default', 'обычный (по умолчанию)'],
  ['blue', 'синий'],
  ['dark', 'тёмный'],
];

export const ratings = [
  ['show', 'показывать'],
  ['hide', 'скрывать'],
];

export const nsfwContents = [
  ['hide', 'не показывать в ленте'],
  ['blur', 'только замазывать'],
  ['show', 'показывать'],
];

export const postViews = [
  ['old', 'старый'],
  ['microblog', 'микроблог'],
];

// timezones are openvk's clocks ("Часовой пояс"), by minutes east of
// UTC; the first is the browser's own.
export const timezones = [
  ['', 'как в браузере (по умолчанию)'],
  ['-720', '(GMT-12): Остров Бейкер, Хауленд'],
  ['-660', '(GMT-11): Паго-Паго, Ниуэ, Мидуэй'],
  ['-600', '(GMT-10): Гонолулу, Папеэте, Адак'],
  ['-540', '(GMT-09): Анкоридж, Фэрбанкс, Джуно'],
  ['-480', '(GMT-08): Лос-Анджелес, Ванкувер, Тихуана'],
  ['-420', '(GMT-07): Денвер, Эдмонтон, Финикс'],
  ['-360', '(GMT-06): Мехико, Чикаго, Гватемала'],
  ['-300', '(GMT-05): Нью-Йорк, Торонто, Гавана'],
  ['-270', '(GMT-04:30): Каракас, Маракайбо'],
  ['-240', '(GMT-04): Сантьяго, Ла-Пас, Галифакс'],
  ['-210', '(GMT-03:30): Сент-Джонс (Ньюфаундленд)'],
  ['-180', '(GMT-03): Буэнос-Айрес, Сан-Паулу, Монтевидео'],
  ['-120', '(GMT-02): Южная Георгия'],
  ['-60', '(GMT-01): Азорские острова, Прая'],
  ['0', '(GMT): Лондон, Лиссабон, Касабланка'],
  ['60', '(GMT+1): Берлин, Париж, Рим'],
  ['120', '(GMT+2): Калининград (МСК-1), Киев, Каир'],
  ['180', '(GMT+3): Москва, Стамбул, Минск'],
  ['210', '(GMT+3:30): Тегеран, Исфахан'],
  ['240', '(GMT+4): Самара (МСК+1), Дубай, Баку'],
  ['270', '(GMT+4:30): Кабул, Кандагар'],
  ['300', '(GMT+5): Екатеринбург (МСК+2), Ташкент, Карачи'],
  ['330', '(GMT+5:30): Нью-Дели, Мумбаи, Коломбо'],
  ['345', '(GMT+5:45): Катманду, Покхара'],
  ['360', '(GMT+6): Омск (МСК+3), Алматы, Дакка'],
  ['390', '(GMT+6:30): Янгон (Рангун), Нейпьидо'],
  ['420', '(GMT+7): Новосибирск (МСК+4), Красноярск, Бангкок'],
  ['480', '(GMT+8): Иркутск (МСК+5), Пекин, Сингапур'],
  ['510', '(GMT+8:30): Пхеньян'],
  ['540', '(GMT+9): Якутск (МСК+6), Токио, Сеул'],
  ['570', '(GMT+9:30): Аделаида, Дарвин'],
  ['600', '(GMT+10): Владивосток (МСК+7), Сидней, Мельбурн'],
  ['630', '(GMT+10:30): Лорд-Хау'],
  ['660', '(GMT+11): Магадан (МСК+8), Сахалин, Чокурдах'],
  ['720', '(GMT+12): Камчатка (МСК+9), Анадырь, Окленд'],
  ['765', '(GMT+12:45): Острова Чатем'],
  ['780', '(GMT+13): Нукуалофа, Апиа'],
  ['840', '(GMT+14): Киритимати (о-ва Лайн)'],
];

export const mainPages = [
  ['page', 'страница'],
  ['feed', 'новости'],
];

export const menuItems = [
  ['photos', 'Фотографии'],
  ['messages', 'Сообщения'],
  ['notes', 'Заметки'],
  ['groups', 'Группы'],
  ['events', 'Встречи'],
  ['feed', 'Новости'],
  ['notifications', 'Ответы'],
];

// deactivationReasons are openvk's reasons to delete a page: a title and
// the words it puts in the message; the last one leaves them to the user.
export const deactivationReasons = [
  ['У меня есть другая страница на сайте', 'Я создал новую страницу и теперь хочу подтереть своё прошлое.'],
  ['Сайт отнимает у меня слишком много времени', 'Пусть этот сайт хорош и прекрасен, но он отнимает у меня время, которое мне нужно для работы и жизни.'],
  ['Сайт содержит слишком много неприемлемых материалов', 'Я нашёл достаточно порнографии и пиратского контента - хватит на всю жизнь. Теперь я ухожу.'],
  ['Меня беспокоит безопасность моих данных', 'За мной следят и мне страшно здесь находится. Извините, я вынужден уйти.'],
  ['Мою страницу не комментируют', 'Меня никто не смотрит здесь и это грустно. Вы пожалеете о том, что я ушёл.'],
  ['Другая причина', ''],
];

// byGender picks the form of a verb for a person; the masculine speaks of
// one whose gender is not set, as on openvk.
function byGender(gender, male, female) {
  return gender === 'female' ? female : male;
}

export function wrote(gender) {
  return byGender(gender, 'написал', 'написала');
}

// updatedPhoto is what openvk says of the post of a new photo (upd_m,
// upd_f, upd_g).
export function updatedPhoto(author) {
  if (author.club) return 'обновило фотографию группы';

  return byGender(author.gender, 'обновил', 'обновила') + ' фотографию на своей странице';
}

// limitedAccess is openvk's "$1 ограничил доступ к своей странице." for
// whoever blacklisted the viewer.
export function limitedAccess(gender) {
  return { male: 'ограничил', female: 'ограничила' }[gender] ?? 'ограничил(а)';
}

// closedFollows says whom the owner of a closed profile lets in.
export function closedFollows(gender) {
  return { male: 'подписан сам', female: 'подписана сама' }[gender] ?? 'подписан(а)';
}

// blacklistCount is the head of the blacklist, as on openvk.
export function blacklistCount(n) {
  if (n <= 0) return 'В вашем чёрном списке ещё нет пользователей.';
  return `В вашем чёрном списке ${n} ${plural(n, ['пользователь', 'пользователя', 'пользователей'])}.`;
}

// sessionsCount heads the list of sessions in the settings.
export function sessionsCount(n) {
  return `Вы вошли на сайт с ${n} ${plural(n, ['устройства', 'устройств', 'устройств'])}.`;
}

const browsers = [
  [/YaBrowser\//, 'Яндекс Браузер'],
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
  if (!agent) return 'Неизвестное устройство';

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
  const at = CASES.indexOf(grammaticalCase);

  return [
    decline(name, firstNameParadigm(name, gender), at),
    decline(surname, surnameParadigm(surname, gender), at),
  ].filter(Boolean).join(' ');
}

export function genitiveName(person) {
  return declinedName(person, 'gen');
}

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

// sameCase writes a stem in capitals when the word it is of is.
function sameCase(word, stem) {
  return word === word.toUpperCase() ? stem.toUpperCase() : stem;
}

export function followersCount(n) {
  return `${n} ${plural(n, ['подписчик', 'подписчика', 'подписчиков'])}`;
}

// onlineCount is openvk's "3 человека онлайн".
export function onlineCount(n) {
  return `${n} ${plural(n, ['человек', 'человека', 'человек'])} онлайн`;
}

// presence is openvk's "Онлайн" or "была в сети сегодня в 12:00" of a
// page, '' when it is not told.
export function presence(user, now = new Date()) {
  if (user.online) return 'Онлайн';
  if (!user.lastSeenAt) return '';

  return `${byGender(user.gender, 'был', 'была')} в сети ${formatPostDate(user.lastSeenAt, now)}`;
}

export function followingCount(n) {
  return `${n} ${plural(n, ['подписка', 'подписки', 'подписок'])}`;
}

// membersCount is openvk's "participants": "Ни одного участника", "3 участника".
export function membersCount(n) {
  return n === 0 ? 'Ни одного участника' : `${n} ${plural(n, ['участник', 'участника', 'участников'])}`;
}

// myClubsCount is the head of "Мои Группы", openvk's "groups_list".
export function myClubsCount(n) {
  if (n === 0) return 'Вы не состоите ни в одной группе';
  return `Вы состоите в ${n} ${plural(n, ['группе', 'группах', 'группах'])}`;
}

// myEventsCount is the head of "Мои Встречи", openvk's "events_list".
export function myEventsCount(n) {
  if (n === 0) return 'Вы не участвуете ни в одной встрече';
  return `Вы участвуете в ${n} ${plural(n, ['встрече', 'встречах', 'встречах'])}`;
}

// eventsCount is openvk's "meetings": "1 встреча", "5 встреч".
export function eventsCount(n) {
  return n === 0 ? 'Ни одной встречи' : `${n} ${plural(n, ['встреча', 'встречи', 'встреч'])}`;
}

// otherComments is openvk's "view_other_comments" under a post of
// "Микроблог": how many comments are on the page of the post only.
export function otherComments(n) {
  return `Показать остальные комментарии (${n})`;
}

// pastEventsCount heads "Прошедшие": the events that are over.
export function pastEventsCount(n) {
  return n === 0 ? 'Ни одной прошедшей встречи' : `${n} ${plural(n, ['прошедшая встреча', 'прошедшие встречи', 'прошедших встреч'])}`;
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

// clubsCount is openvk's "groups": "1 группа", "5 групп".
export function clubsCount(n) {
  return n === 0 ? 'Ни одной группы' : `${n} ${plural(n, ['группа', 'группы', 'групп'])}`;
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
  return n === 0 ? 'В теме нет сообщений' : `В теме ${topicMessages(n)}`;
}

// resultsCount is openvk's "results": "Ни одного результата", "2 результата".
export function resultsCount(n) {
  return n === 0 ? 'Ни одного результата' : `${n} ${plural(n, ['результат', 'результата', 'результатов'])}`;
}

// suggestedInClub is the head of the suggestions of a club, openvk's
// "suggested_posts_in_group" and, for your own, "..._by_you"; at 0, once
// all are handled.
export function suggestedInClub(n, mine) {
  if (n === 0) return mine ? 'Вы не предлагали в эту группу никаких записей' : 'Вы посмотрели всю предложку, поздравляю!';

  const posts = `${n} ${plural(n, ['запись', 'записи', 'записей'])}`;
  return mine ? `Вы предложили в эту группу ${posts}` : `В эту группу предложили ${posts}`;
}

// albumsCount is openvk's "albums": "Ни одного альбома", "3 альбома".
export function albumsCount(n) {
  return n === 0 ? 'Ни одного альбома' : `${n} ${plural(n, ['альбом', 'альбома', 'альбомов'])}`;
}

// myAlbumsCount is the head of your own albums, openvk's "albums_list".
export function myAlbumsCount(n) {
  return n === 0 ? 'У Вас нет ни одного альбома' : `У Вас ${albumsCount(n)}`;
}

// Gifts ("Подарки" of openvk): the collections there are to choose from,
// what each gift is called and its picture. The gifts themselves are the
// catalogue of the user service; a picture is a file of the site, named
// by its whole path so that it is served with its version.
export const giftCollections = { flowers: 'Цветы', sweets: 'Сладости', holidays: 'Праздники' };

export const giftNames = {
  rose: 'Роза', tulip: 'Тюльпан', bouquet: 'Букет', sunflower: 'Подсолнух', cactus: 'Кактус',
  cake: 'Торт', candy: 'Конфета', chocolate: 'Шоколад', icecream: 'Мороженое', cupcake: 'Пирожное',
  present: 'Подарок', balloons: 'Шарики', heart: 'Сердце', bear: 'Мишка', star: 'Звезда',
};

const giftImages = {
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

// giftsCount is openvk's "gifts": "3 подарка", "Нет подарков".
export function giftsCount(n) {
  return n === 0 ? 'Нет подарков' : `${n} ${plural(n, ['подарок', 'подарка', 'подарков'])}`;
}

// notesCount is openvk's "notes" of the block on a page: "3 заметки";
// foundNotes heads the list of notes, "Найдено 3 заметки".
export function notesCount(n) {
  return n === 0 ? 'Ни одной заметки' : `${n} ${plural(n, ['заметка', 'заметки', 'заметок'])}`;
}

// favesCount is openvk's "N закладок" above what you like.
export function favesCount(n) {
  return n === 0 ? 'Ни одной закладки' : `${n} ${plural(n, ['закладка', 'закладки', 'закладок'])}`;
}

export function foundNotes(n) {
  return n === 0 ? 'Ни одной заметки' : `${plural(n, ['Найдена', 'Найдено', 'Найдено'])} ${notesCount(n)}`;
}

// photosCount is under an album: openvk writes "N фотографий" whatever N
// ("TODO: Добавить склонения"); we decline it.
export function photosCount(n) {
  return `${n} ${plural(n, ['фотография', 'фотографии', 'фотографий'])}`;
}

// clubRoles name what a member is in a community.
export const clubRoles = { owner: 'Владелец', admin: 'Администратор', member: 'Участник' };

export function postsCount(n) {
  return `${n} ${plural(n, ['запись', 'записи', 'записей'])}`;
}

// likedBy is openvk's "Понравилось 3 людям".
export function likedBy(n) {
  return n === 0 ? 'Никому не понравилось' : `Понравилось ${n} ${plural(n, ['человеку', 'людям', 'людям'])}`;
}

// sharedBy heads the list of who shared a post: "Поделились 3 человека".
export function sharedBy(n) {
  if (n === 0) return 'Никто не поделился';

  return `${plural(n, ['Поделился', 'Поделились', 'Поделились'])} ${n} ${plural(n, ['человек', 'человека', 'человек'])}`;
}

// pollVoters is openvk's line under a poll: how many people voted.
export function pollVoters(n) {
  if (n === 0) return 'Будьте первым, кто проголосует!';

  return `В опросе ${plural(n, ['проголосовал', 'проголосовало', 'проголосовало'])} ${n} ${plural(n, ['человек', 'человека', 'человек'])}.`;
}

// pollMeta is openvk's second line: how the poll is held, its parts
// joined with " • ": "Публичное голосование • много вариантов • нельзя
// переголосовать • до 12 октября в 15:30".
export function pollMeta(poll, now = new Date()) {
  const parts = [poll.anonymous ? 'Анонимное голосование' : 'Публичное голосование'];
  if (poll.multiple) parts.push('много вариантов');
  if (poll.locked) parts.push('нельзя переголосовать');
  if (poll.endsAt) parts.push('до ' + formatPostDate(poll.endsAt, now));

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
  return n === 0 ? 'нет записей' : postsCount(n);
}

// archiveYears lays out the years of openvk's "Архив записей", the
// latest first: the first six are links, the rest go under "Ещё". chosen
// is the year the archive shows, 0 for "Все годы"; the year "Действия"
// are about is the chosen one or, without it, the latest.
export function archiveYears(years, chosen = 0, shown = 6) {
  const all = years.includes(chosen) || !chosen ? years : [...years, chosen].sort((a, b) => b - a);

  return { shown: all.slice(0, shown), more: all.slice(shown), action: chosen || all[0] || 0 };
}

// commonCount is openvk's "3 общих друга", of the people followed.
export function commonCount(n) {
  return `${n} ${plural(n, ['общая подписка', 'общие подписки', 'общих подписок'])}`;
}

// completenessHints are openvk's hints under the gauge of the owner's page:
// what is missing, where to fill it in and what it adds.
export const completenessHints = {
  interests: ['/edit?act=interests', 'Интересы (+20%)'],
  email: ['/edit?act=contacts', 'Email (+20%)'],
  telegram: ['/edit?act=contacts', 'Telegram (+15%)'],
  status: ['/edit', 'Статус (+15%)'],
};

// Notification phrases, as on openvk: "<name> оценил(а) вашу запись" and
// so on; the verb agrees with the actor's gender, or is neuter for a
// community: "Сообщество Кошки оставило комментарий".
const notificationVerbs = {
  like: ['оценил', 'оценила', 'оценил(а)', 'оценило'],
  comment: ['оставил', 'оставила', 'оставил(а)', 'оставило'],
  follow: ['подписался', 'подписалась', 'подписался(-ась)', 'подписалось'],
  wall_post: ['написал', 'написала', 'написал(а)', 'написало'],
  repost: ['поделился', 'поделилась', 'поделился(-лась)', 'поделилось'],
  comment_reply: ['ответил', 'ответила', 'ответил(а)', 'ответило'],
  comment_like: ['оценил', 'оценила', 'оценил(а)', 'оценило'],
  mention: ['упомянул', 'упомянула', 'упомянул(а)', 'упомянуло'],
  gift: ['отправил', 'отправила', 'отправил(а)', 'отправило'],
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

// commentsLink is the link to the comments under a post, as on openvk.
export function commentsLink(n) {
  return n > 0 ? `Комментарии (${n})` : 'Комментировать';
}

// shareLink is openvk's "Поделиться" with how many times it was shared.
export function shareLink(n) {
  return n > 0 ? `Поделиться (${n})` : 'Поделиться';
}

const errorTexts = {
  'network error': 'Нет связи с сервером. Проверьте подключение.',
  'invalid email': 'Похоже, в адресе почты опечатка.',
  'invalid or expired code': 'Неверный или устаревший код.',
  'too many requests, try again later': 'Слишком часто. Подождите немного и попробуйте снова.',
  'poll not found': 'Опрос не найден.',
  'poll title is empty': 'Укажите тему опроса.',
  'poll title is too long': 'Тема опроса длиннее 128 символов.',
  'poll option is empty': 'Вариант ответа не может быть пустым.',
  'poll option is too long': 'Вариант ответа длиннее 128 символов.',
  'a poll has at least one option': 'Добавьте хотя бы один вариант ответа.',
  'a poll has at most 10 options': 'Слишком много вариантов в опросе.',
  'a poll is open for 1 to 31 days, or for good': 'Опрос может длиться от 1 до 31 дня.',
  'invalid choice of poll options': 'Выберите вариант ответа.',
  'the poll has ended': 'Голосование окончено.',
  'you have voted in the poll already': 'Вы уже проголосовали в этом опросе.',
  'you have not voted in the poll': 'Вы не голосовали в этом опросе.',
  'the poll does not let a vote be taken back': 'В этом опросе нельзя отменить голос.',
  'the poll is anonymous': 'Невозможно просмотреть список проголосовавших в анонимном голосовании.',
  'the poll of a post cannot be changed': 'Опрос записи изменить нельзя.',
  'too many attempts, request a new code': 'Слишком много неверных попыток. Запросите новый код.',
  'name is empty': 'Укажите имя.',
  'name is too long': 'Имя слишком длинное.',
  'surname is too long': 'Фамилия слишком длинная.',
  'nickname is empty': 'Придумайте никнейм.',
  'nickname is too long': 'Никнейм длиннее 32 символов.',
  'nickname contains invalid characters': 'Никнейм: латинские буквы, цифры и «_», начинается с буквы, не заканчивается «_».',
  'nickname is reserved': 'Этот никнейм зарезервирован, выберите другой.',
  'nickname is already taken': 'Этот никнейм уже занят.',
  'status is too long': 'Статус длиннее 200 символов.',
  'birthday is in the future': 'Дата рождения не может быть в будущем.',
  'user not found': 'Страница не найдена.',
  'session not found': 'Эта сессия уже завершена.',
  'post text is empty': 'Напишите что-нибудь.',
  'post text is too long': 'Запись длиннее 60 000 символов.',
  'post not found': 'Запись уже удалена.',
  'the post can no longer be edited': 'Запись можно редактировать только в течение недели после публикации.',
  'a post sharing a photo has no photos of its own': 'К записи с фотографией из альбома нельзя прикрепить другие фотографии.',
  'a shared photo goes with text alone': 'Фотографию из альбома можно отправить только с текстом.',
  'photo not found': 'Фотография не найдена, прикрепите её снова.',
  'a post has at most 10 photos': 'К записи можно прикрепить не больше 10 фотографий.',
  'the same photo is attached twice': 'Эта фотография уже прикреплена.',
  'fill in your profile before posting': 'Сначала заполните свою страницу.',
  'this club is closed': 'Это закрытая группа. Для вступления необходимо подавать заявку.',
  'join request not found': 'Заявка не найдена.',
  'you cannot post on this wall': 'Хозяин страницы не разрешает вам писать у него на стене.',
  'you cannot add this user to a chat': 'Пользователь запретил добавлять себя в беседы.',
  'a chat needs at least two other people': 'В беседе должно быть как минимум два других пользователя.',
  'the chat is full': 'В беседе не может быть больше 50 человек.',
  'you are not in this chat': 'Вы не состоите в этой беседе.',
  'the user has left the chat': 'Пользователь сам покинул беседу и может вернуться только сам.',
  'you were removed from this chat': 'Вас исключили из беседы: вернуться можно, только если вас снова пригласят.',
  'you cannot remove yourself, leave the chat instead': 'Себя нельзя исключить — покиньте чат.',
  'chat not found': 'Такой беседы нет.',
  'member not found': 'Этого человека нет в беседе.',
  'you cannot write to this user': 'Вы не можете отправить сообщение этому пользователю, поскольку он ограничил круг лиц, которые могут присылать ему сообщения.',
  'you cannot comment on this post': 'Вы не можете комментировать эту запись.',
  'topic title is empty': 'Заголовок не указан.',
  'topic title is too long': 'Заголовок длиннее 127 символов.',
  'topic not found': 'Тема не найдена или удалена.',
  'message not found': 'Сообщение уже удалено.',
  'you cannot start topics here': 'Создавать темы в этой группе могут только администраторы.',
  'you cannot write for this community': 'Писать от имени сообщества могут только его администраторы.',
  'the topic is closed': 'Тема закрыта.',
  'only a topic of the community can be restricted': 'Ограничить комментарии можно только в теме от имени сообщества.',
  'album name is empty': 'Укажите название альбома.',
  'album name is too long': 'Название альбома длиннее 36 символов.',
  'description is too long': 'Описание слишком длинное.',
  'album not found': 'Альбом не найден или удалён.',
  'no photos to add': 'Загрузите хотя бы одну фотографию.',
  'at most 10 photos are added at once': 'Вы можете загружать до 10 фотографий за один раз.',
  'fill in your profile before creating albums': 'Сначала заполните свою страницу.',
  'fill in your profile before uploading photos': 'Сначала заполните свою страницу.',
  'partner not found': 'Пользователь с таким никнеймом не найден.',
  'you cannot name yourself in your relationship': 'Нельзя указать в семейном положении себя.',
  'place is too long': 'Название города длиннее 64 символов.',
  'invalid telegram username': 'Неверное имя пользователя Telegram.',
  'website is not a link': 'Укажите адрес сайта, например example.com.',
  'website is too long': 'Адрес сайта длиннее 256 символов.',
  'address is too long': 'Адрес длиннее 255 символов.',
  'text is too long': 'Текст длиннее 60 000 символов.',
  'at most 7 additional fields': 'Можно добавить не больше 7 полей.',
  'field name is empty': 'Укажите название поля.',
  'field name is too long': 'Название поля длиннее 50 символов.',
  'field text is empty': 'Заполните текст поля.',
  'you cannot blacklist yourself': 'Нельзя внести в чёрный список самого себя.',
  'you cannot ignore yourself': 'Нельзя игнорировать самого себя.',
  'too many ignored sources': 'Превышен лимит игнорируемых источников.',
  'invalid post id': 'Запись не найдена.',
  'comment text is empty': 'Напишите что-нибудь.',
  'comment text is too long': 'Комментарий длиннее 60 000 символов.',
  'comment not found': 'Комментарий уже удалён.',
  'fill in your profile before commenting': 'Сначала заполните свою страницу.',
  'notification not found': 'Уведомление уже удалено.',
  'message text is empty': 'Напишите что-нибудь.',
  'message text is too long': 'Сообщение слишком длинное.',
  'message not found': 'Сообщение не найдено.',
  'club not found': 'Группа не найдена.',
  'club name is empty': 'Укажите название группы.',
  'club name is too long': 'Название группы длиннее 64 символов.',
  'club description is too long': 'Описание длиннее 1000 символов.',
  'club address is too long': 'Адрес страницы длиннее 32 символов.',
  'club address contains invalid characters': 'Адрес страницы может состоять из латинских букв, цифр и _ и начинаться с буквы.',
  'club address is already taken': 'Этот адрес уже занят.',
  'the owner cannot leave the club': 'Владелец не может выйти из своей группы.',
  'invalid avatar style': 'Неизвестный вид аватаров.',
  'invalid main page': 'Неизвестная главная страница.',
  'invalid menu item': 'Неизвестный пункт меню.',
  'invalid theme': 'Неизвестный стиль.',
  'invalid nsfw setting': 'Неизвестная настройка NSFW-контента.',
  'invalid view of posts': 'Неизвестный вид постов.',
  'invalid timezone': 'Неизвестный часовой пояс.',
  'title is too long': 'Комментарий слишком длинный: не больше 36 символов.',
  'an event needs the time it starts': 'Вы не ввели название встречи и/или не указали дату начала встречи.',
  'an event ends after it starts': 'Встреча должна заканчиваться позже, чем начинается.',
  'invalid start time': 'Неверная дата начала встречи.',
  'invalid end time': 'Неверная дата окончания встречи.',
  'the user does not run the club': 'Этот человек не руководит группой.',
  'only the admins can add the club to their menu': 'Добавить группу в левое меню могут только её администраторы.',
  'at most 10 clubs in the menu': 'В левом меню может быть не больше 10 групп.',
  'only the admins can edit the club': 'Изменять группу могут только её администраторы.',
  'club website is too long': 'Адрес сайта длиннее 256 символов.',
  'club website is not a link': 'Укажите адрес сайта, например example.com.',
  'the new owner must be an admin of the club': 'Владельцем можно назначить только администратора группы.',
  "this user's clubs are hidden": 'Пользователь скрыл список своих групп.',
  "this user's photos are hidden": 'Пользователь скрыл свои фотографии.',
  "this user's notes are hidden": 'Пользователь скрыл свои заметки.',
  "this user's gifts are hidden": 'Пользователь скрыл свои подарки.',
  'you cannot send a gift to this user': 'Вы не можете отправить подарок этому пользователю.',
  'fill in your profile before sending gifts': 'Сначала заполните свою страницу.',
  'unknown gift': 'Такого подарка нет.',
  'gift caption is too long': 'Сообщение слишком длинное: не больше 250 символов.',
  'gift not found': 'Подарок не найден.',
  'note not found': 'Заметка не найдена или удалена.',
  'note title is empty': 'Введите название заметки.',
  'note title is too long': 'Название заметки слишком длинное: не больше 256 символов.',
  'note text is empty': 'Заметка не может быть пустой.',
  'note text is too long': 'Заметка слишком длинная: не больше 60 000 символов.',
  'fill in your profile before writing notes': 'Сначала заполните свою страницу.',
  'you cannot comment on this note': 'Вы не можете комментировать эту заметку.',
  'only the sender can edit the message': 'Редактировать можно только свои сообщения.',
  'the message can no longer be edited': 'Сообщение можно редактировать только в течение суток после отправки.',
  'forwarded message not found': 'Сообщение не удалось переслать: его уже нет.',
  'a message forwards at most 20 messages': 'За один раз можно переслать не больше 20 сообщений.',
  'search query is empty': 'Введите, что искать.',
  'the comments are closed': 'Комментарии к этой записи закрыты.',
  'search query is too long': 'Запрос длиннее 100 символов.',
  'this album is kept by the site': 'Этот альбом сайт ведёт сам: его нельзя изменить.',
  'a photo moves between the albums of one owner': 'Фотографию можно перенести только в другой альбом той же страницы.',
  "this user's follows are hidden": 'Пользователь скрыл список своих подписок.',
  'this page is hidden': 'Настройки приватности этого пользователя не разрешают вам смотреть на его страницу.',
  'you cannot suggest posts here': 'В эту группу нельзя предлагать записи.',
  'suggestion not found': 'Запись уже опубликована или отклонена.',
  'fill in your profile before creating a club': 'Сначала заполните свою страницу.',
  'fill in your profile before joining a club': 'Сначала заполните свою страницу.',
  'the message can no longer be deleted': 'Сообщение можно удалить только в течение суток после отправки.',
  'you cannot write to yourself': 'Нельзя написать самому себе.',
  'fill in your profile before writing messages': 'Сначала заполните свою страницу.',
  'permission denied': 'Недостаточно прав.',
  'this page is deleted': 'Страница пользователя заблокирована или удалена.',
  'this page is not deleted': 'Ваша страница не удалена.',
  'this page can no longer be restored': 'Срок восстановления страницы истёк.',
  'reason is too long': 'Сообщение длиннее 1000 символов.',
  'you cannot follow yourself': 'Нельзя подписаться на себя.',
  'image is larger than 8 MB': 'Фотография больше 8 МБ.',
  'not a JPEG, PNG, GIF or WebP image': 'Это не фотография: подойдут JPEG, PNG, GIF и WebP.',
  'image dimensions are too large': 'Слишком большое разрешение фотографии.',
  'image is smaller than 50x50': 'Фотография меньше 50×50 точек.',
  'search query is empty': 'Введите имя, фамилию или никнейм.',
  'search query is too long': 'Слишком длинный запрос.',
  'fill in your profile before following': 'Сначала заполните свою страницу.',
  'service temporarily unavailable': 'Сервис временно недоступен. Попробуйте чуть позже.',
  'missing access token': 'Сессия истекла, войдите снова.',
  'invalid access token': 'Сессия истекла, войдите снова.',
  'invalid refresh token': 'Сессия истекла, войдите снова.',
};

export function errorText(e) {
  return errorTexts[e?.message] ?? 'Что-то пошло не так. Попробуйте позже.';
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
  return n === 1 ? 'Пересланное сообщение' : `${n} ${plural(n, ['пересланное сообщение', 'пересланных сообщения', 'пересланных сообщений'])}`;
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
  if (m.deleted) return 'Сообщение удалено';
  if (m.text) return plainMentions(m.text);
  if (m.photos?.length || m.albumPhoto) return 'Фотография';
  if (m.post) return 'Запись';
  if (m.forwardedCount) return forwardedCount(m.forwardedCount);

  return '';
}

// Chats of several people ("беседы"). A chat without a title of its own
// is "Беседа"; a new one left untitled is named by who is in it, as VK
// names them: "Анна, Пётр, Иван", at most 64 characters.
export const CHAT_TITLE_MAX = 64;

export function chatTitle(chat) {
  return chat.title || 'Беседа';
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
      return [`${byGender(g, 'создал', 'создала')} беседу` + (m.text ? ` «${m.text}»` : ''), '', ''];
    case 'title':
      return [m.text
        ? `${byGender(g, 'изменил', 'изменила')} название беседы на «${m.text}»`
        : `${byGender(g, 'убрал', 'убрала')} название беседы`, '', ''];
    case 'invite':
      return [`${byGender(g, 'пригласил', 'пригласила')} `, whom, ''];
    case 'kick':
      return [`${byGender(g, 'исключил', 'исключила')} `, whom, ' из беседы'];
    case 'leave':
      return [`${byGender(g, 'покинул', 'покинула')} беседу`, '', ''];
    case 'return':
      return [`${byGender(g, 'вернулся', 'вернулась')} в беседу`, '', ''];
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
  return state === 'kicked' ? 'Вас исключили из беседы.' : 'Вы покинули беседу.';
}

// selectedCount is over a dialog while messages are selected: "Выбрано 2
// сообщения".
export function selectedCount(n) {
  return `Выбрано ${n} ${plural(n, ['сообщение', 'сообщения', 'сообщений'])}`;
}

// "Настройки ленты" of openvk: how many posts a page of the news holds
// (its choices) and whether what people wrote on the walls of others is
// there. They are kept by the browser, as openvk keeps them in a cookie.
export const feedPageSizes = [1, 5, 10, 20, 30, 40, 50];

export const defaultFeedSettings = Object.freeze({ posts: 20, alien: true });

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
// пользователя" and "Игнорировать группу" (ours for an event).
export function ignoreLabel(kind, ignored) {
  const whom = { user: 'пользователя', club: 'группу', event: 'встречу' }[kind];

  return (ignored ? 'Не игнорировать ' : 'Игнорировать ') + whom;
}
