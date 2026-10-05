// Russian texts for API values and errors.

export const genders = [
  ['', 'не указан'],
  ['male', 'мужской'],
  ['female', 'женский'],
];

// Relationship labels agree with gender, as in Russian they must.
const relationshipLabels = {
  single: { male: 'не женат', female: 'не замужем', '': 'не женат / не замужем' },
  dating: { male: 'встречается', female: 'встречается', '': 'встречается' },
  married: { male: 'женат', female: 'замужем', '': 'женат / замужем' },
  in_love: { male: 'влюблён', female: 'влюблена', '': 'влюблён(а)' },
  looking: { male: 'в активном поиске', female: 'в активном поиске', '': 'в активном поиске' },
};

export function relationships(gender) {
  return [['', 'не выбрано'], ...Object.entries(relationshipLabels).map(([value, byGender]) => [value, byGender[gender] ?? byGender['']])];
}

export const worldviews = [
  ['', 'не выбрано'],
  ['orthodox', 'Православие'],
  ['catholicism', 'Католицизм'],
  ['protestantism', 'Протестантизм'],
  ['islam', 'Ислам'],
  ['judaism', 'Иудаизм'],
  ['buddhism', 'Буддизм'],
  ['confucianism', 'Конфуцианство'],
  ['secular_humanism', 'Светский гуманизм'],
];

export const politicalViews = [
  ['', 'не выбраны'],
  ['indifferent', 'индифферентные'],
  ['communist', 'коммунистические'],
  ['socialist', 'социалистические'],
  ['moderate', 'умеренные'],
  ['liberal', 'либеральные'],
  ['conservative', 'консервативные'],
  ['monarchist', 'монархические'],
  ['ultraconservative', 'ультраконсервативные'],
  ['libertarian', 'либертарианские'],
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
export function formatBirthday(iso) {
  const [year, month, day] = iso.split('-').map(Number);
  const today = new Date();

  let age = today.getFullYear() - year;
  if (today.getMonth() + 1 < month || (today.getMonth() + 1 === month && today.getDate() < day)) age--;

  return `${day} ${months[month - 1]} ${year} г., ${age} ${plural(age, ['год', 'года', 'лет'])}`;
}

// formatPostDate turns an ISO time into "сегодня в 9:05", "вчера в 18:30",
// "17 мая в 10:00" or, for other years, "17 мая 2025 в 10:00".
export function formatPostDate(iso, now = new Date()) {
  const date = new Date(iso);
  const time = `${date.getHours()}:${String(date.getMinutes()).padStart(2, '0')}`;

  const midnight = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
  // Rounded, so a daylight-saving shift does not turn a day into 0.96.
  const daysAgo = Math.round((midnight(now) - midnight(date)) / 86_400_000);

  if (daysAgo === 0) return `сегодня в ${time}`;
  if (daysAgo === 1) return `вчера в ${time}`;

  const day = `${date.getDate()} ${months[date.getMonth()]}`;

  return date.getFullYear() === now.getFullYear() ? `${day} в ${time}` : `${day} ${date.getFullYear()} в ${time}`;
}

export function wrote(gender) {
  return gender === 'female' ? 'написала' : 'написал';
}

export function followersCount(n) {
  return `${n} ${plural(n, ['подписчик', 'подписчика', 'подписчиков'])}`;
}

export function followingCount(n) {
  return `${n} ${plural(n, ['подписка', 'подписки', 'подписок'])}`;
}

export function postsCount(n) {
  return `${n} ${plural(n, ['запись', 'записи', 'записей'])}`;
}

// commentsLink is the link to the comments under a post, as on openvk.
export function commentsLink(n) {
  return n > 0 ? `Комментарии (${n})` : 'Комментировать';
}

const errorTexts = {
  'network error': 'Нет связи с сервером. Проверьте подключение.',
  'invalid email': 'Похоже, в адресе почты опечатка.',
  'invalid or expired code': 'Неверный или устаревший код.',
  'too many requests, try again later': 'Слишком часто. Подождите минуту и попробуйте снова.',
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
  'post text is empty': 'Напишите что-нибудь.',
  'post text is too long': 'Запись длиннее 1000 символов.',
  'post not found': 'Запись уже удалена.',
  'fill in your profile before posting': 'Сначала заполните свою страницу.',
  'invalid post id': 'Запись не найдена.',
  'comment text is empty': 'Напишите что-нибудь.',
  'comment text is too long': 'Комментарий длиннее 1000 символов.',
  'comment not found': 'Комментарий уже удалён.',
  'fill in your profile before commenting': 'Сначала заполните свою страницу.',
  'permission denied': 'Недостаточно прав.',
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
