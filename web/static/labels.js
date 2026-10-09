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

// wallWrites is "Кто может писать у меня на стене" of openvk. There are no
// friends here: openvk's "Друзья" are the people you follow.
export const wallWrites = [
  ['everyone', 'Все желающие'],
  ['following', 'Те, на кого я подписан(а)'],
  ['only_me', 'Только я'],
];

// messagesWrites is "Кто может писать мне сообщения" of openvk, with its
// "Друзья" as the people you follow; whoever you wrote to may answer.
export const messagesWrites = [
  ['everyone', 'Все желающие'],
  ['following', 'Те, на кого я подписан(а)'],
  ['nobody', 'Никто'],
];

// clubsReads is "Кому видно мои группы" of openvk, with its "Друзьям" as
// the people you follow.
export const clubsReads = [
  ['everyone', 'Всем желающим'],
  ['following', 'Тем, на кого я подписан(а)'],
  ['only_me', 'Только мне'],
];

// adminsDisplays is "Список админов" of a community's settings in openvk.
export const adminsDisplays = [
  ['creator', 'Отображать только создателя группы'],
  ['all', 'Отображать всех администраторов'],
  ['none', 'Ничего не отображать'],
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

// formatDateTime turns an ISO time into openvk's "7 мая 2027 в 12:00".
export function formatDateTime(iso) {
  const date = new Date(iso);
  const time = `${date.getHours()}:${String(date.getMinutes()).padStart(2, '0')}`;

  return `${date.getDate()} ${months[date.getMonth()]} ${date.getFullYear()} в ${time}`;
}

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

export function wrote(gender) {
  return gender === 'female' ? 'написала' : 'написал';
}

// updatedPhoto is what openvk says of the post of a new photo (upd_m,
// upd_f, upd_g).
export function updatedPhoto(author) {
  if (author.club) return 'обновило фотографию группы';

  return author.gender === 'female' ? 'обновила фотографию на своей странице' : 'обновил фотографию на своей странице';
}

// limitedAccess is openvk's "$1 ограничил доступ к своей странице." for
// whoever blacklisted the viewer.
export function limitedAccess(gender) {
  return { male: 'ограничил', female: 'ограничила' }[gender] ?? 'ограничил(а)';
}

// blacklistCount is the head of the blacklist, as on openvk.
export function blacklistCount(n) {
  if (n <= 0) return 'В вашем чёрном списке ещё нет пользователей.';
  return `В вашем чёрном списке ${n} ${plural(n, ['пользователь', 'пользователя', 'пользователей'])}.`;
}

// genitiveName is "Петра Петрова" for "на стене Петра Петрова", as openvk
// says it. Russian names are declined by their endings; whatever the
// rules do not know (Latin letters, Шевченко, Кармен) stays as it is.
export function genitiveName({ name = '', surname = '', gender = '' }) {
  return [genitiveFirstName(name, gender), genitiveSurname(surname, gender)].filter(Boolean).join(' ');
}

const CYRILLIC = /^[а-яё-]+$/i;
// After these letters Russian writes и, not ы: Ольга → Ольги.
const SOFT_BEFORE_A = /[гкхжчшщ]а$/i;
// Names that lose a vowel or a dot: Павел → Павла.
const irregularNames = { павел: 'Павла', лев: 'Льва', пётр: 'Петра', петр: 'Петра' };

function genitiveFirstName(name, gender) {
  const lower = name.toLowerCase();
  if (!CYRILLIC.test(name)) return name;
  if (irregularNames[lower]) return irregularNames[lower];

  return declineEnding(name, gender);
}

function genitiveSurname(surname, gender) {
  if (!CYRILLIC.test(surname)) return surname;

  // Female forms tell themselves apart even when gender is not set.
  if (/(ов|ев|ёв|ин|ын)а$/i.test(surname)) return replaceEnd(surname, 1, 'ой');
  if (/ая$/i.test(surname)) return replaceEnd(surname, 2, 'ой');
  if (gender === 'female') return surname;

  if (/(ий|ый|ой)$/i.test(surname)) return replaceEnd(surname, 2, 'ого');
  if (/(ых|их)$/i.test(surname)) return surname;

  return declineEnding(surname, gender);
}

// declineEnding declines a noun-like name or surname by its last letters.
function declineEnding(word, gender) {
  if (/ия$/i.test(word)) return replaceEnd(word, 1, 'и');
  if (/я$/i.test(word)) return replaceEnd(word, 1, 'и');
  if (/а$/i.test(word)) return replaceEnd(word, 1, SOFT_BEFORE_A.test(word) ? 'и' : 'ы');
  if (/[оеиуюэы]$/i.test(word)) return word;
  if (gender === 'female') return /ь$/i.test(word) ? replaceEnd(word, 1, 'и') : word;
  if (/[йь]$/i.test(word)) return replaceEnd(word, 1, 'я');

  return replaceEnd(word, 0, 'а');
}

// replaceEnd swaps the last n letters, keeping capitals of a word written
// in capitals (ИВАНОВ → ИВАНОВА).
function replaceEnd(word, n, ending) {
  const base = n > 0 ? word.slice(0, -n) : word;
  return base + (word === word.toUpperCase() ? ending.toUpperCase() : ending);
}

export function followersCount(n) {
  return `${n} ${plural(n, ['подписчик', 'подписчика', 'подписчиков'])}`;
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
  'the post can no longer be edited': 'Запись можно редактировать только в течение недели после публикации.',
  'a repost has no photos of its own': 'К репосту нельзя прикрепить фотографии.',
  'photo not found': 'Фотография не найдена, прикрепите её снова.',
  'a post has at most 10 photos': 'К записи можно прикрепить не больше 10 фотографий.',
  'the same photo is attached twice': 'Эта фотография уже прикреплена.',
  'fill in your profile before posting': 'Сначала заполните свою страницу.',
  'you cannot post on this wall': 'Хозяин страницы не разрешает вам писать у него на стене.',
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
  'you cannot blacklist yourself': 'Нельзя внести в чёрный список самого себя.',
  'invalid post id': 'Запись не найдена.',
  'comment text is empty': 'Напишите что-нибудь.',
  'comment text is too long': 'Комментарий длиннее 1000 символов.',
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
  'only the admins can add the club to their menu': 'Добавить группу в левое меню могут только её администраторы.',
  'at most 10 clubs in the menu': 'В левом меню может быть не больше 10 групп.',
  'only the admins can edit the club': 'Изменять группу могут только её администраторы.',
  'club website is too long': 'Адрес сайта длиннее 256 символов.',
  'club website is not a link': 'Укажите адрес сайта, например example.com.',
  'the new owner must be an admin of the club': 'Владельцем можно назначить только администратора группы.',
  "this user's clubs are hidden": 'Пользователь скрыл список своих групп.',
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
