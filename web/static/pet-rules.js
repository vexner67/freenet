// The tab "Правила" of the pets: all the rules and mechanics at one place,
// so that the other tabs keep to names, numbers and buttons.

import { formatDateTime, groupedVotes, petCaseName, petCaseText, petElementName, petKindName, petKindText, petKinds, petMoveName, petMoveText, petMoves, petRarityName, petSpecialName, petSpecialText, petStanceName, petStanceText, petStances, petStatName, petStatText, petStats, votesAmount } from './labels.js';
import { el } from './layout.js';
import { marketFee, minPrices, petWindow } from './pet-league.js';
import { petImage } from './pet.js';
import { tr } from './i18n.js';

const text = (words) => el('p', { textContent: words });

// terms is a list of name and meaning.
function terms(rows) {
  return el('table', { className: 'info pet-rules-table' }, ...rows.map(([name, about]) => el('tr', {},
    el('td', { className: 'label', textContent: name }), el('td', { textContent: about }))));
}

// breeds are all the kinds with what sets each apart.
function breeds(catalogue) {
  return el('table', { className: 'info pet-rules-table' }, ...petKinds.map((kind) => {
    const of = catalogue.kinds.find((k) => k.kind === kind);
    const trick = of?.special ? `${petSpecialName(of.special)} (${petElementName(of.element)}): ${petSpecialText(of.special)}` : '';

    return el('tr', {},
      el('td', { className: 'pet-rules-pet' }, petImage(kind, 32)),
      el('td', {}, el('b', { textContent: petKindName(kind) }), ' ', el('span', { className: 'hint', textContent: of ? petRarityName(of.rarity) : '' }),
        el('div', { textContent: petKindText(kind) }), trick ? el('div', { className: 'hint', textContent: trick }) : null));
  }));
}

// rulesNodes are the windows of the tab; arena gives the numbers of the
// boss and the season, catalogue the kinds and the cases.
export function rulesNodes(arena, catalogue) {
  const boss = arena.boss;

  return [
    petWindow('/static/icons/pet/shop.svg', tr('Как завести питомца'), [
      text(tr('Заведите питомцев — как тамагочи из детства, до десяти сразу. Кормите их и играйте с ними, иначе они загрустят. С каждым уходом и боем питомец набирается опыта, растёт в уровне и становится сильнее. Вызывайте на бой питомцев других людей: победа приносит голоса и рейтинг.')),
    ]),
    petWindow('/static/icons/pet/case.svg', tr('Кейсы'), [
      text(tr('В магазине продаются не все питомцы: лис, сова, волк, панда, единорог и феникс выпадают только из кейсов. Чем дороже кейс, тем чаще из него выходят редкие питомцы, а каждый сороковой кейс без легендарного даёт легендарного наверняка. Выпавшего питомца можно не забирать: тогда он оставит осколки, как и отпущенный. За осколки тоже открываются кейсы.')),
      terms(catalogue.cases.map((c) => [petCaseName(c.id), petCaseText(c.id)])),
    ]),
    petWindow('/static/icons/pet/care.svg', tr('Уход'), [
      text(tr('Покормить можно, пока питомец не сыт. Поиграть можно раз в 10 минут. Час отдыха снимает усталость от боёв подряд: каждый бой подряд отнимает 5% силы.')),
    ]),
    petWindow('/static/icons/pet/wardrobe.svg', tr('Гардероб'), [
      text(tr('Одежда прибавляет питомцу силы. Наведите на вещь, чтобы примерить её; купленная остаётся в гардеробе, её можно снять и надеть снова. В каждое место — одна вещь.')),
    ]),
    petWindow('/static/icons/pet/stats.svg', tr('Характеристики'), [terms(petStats.map((stat) => [petStatName(stat), petStatText(stat)]))]),
    petWindow('/static/icons/pet/fight.svg', tr('Бой'), [
      text(tr('Победа над равным приносит голоса и рейтинг, над сильным — больше, над слабым — меньше; соперника подбирают по силе. Для боя питомцу нужны сытость и настроение. Выставленный на рынок питомец не дерётся.')),
      terms(petStances.map((stance) => [petStanceName(stance), petStanceText(stance)])),
      terms(petMoves.map((move) => [petMoveName(move), petMoveText(move)])),
    ]),
    petWindow('/static/icons/pet/quests.svg', tr('Задания дня'), [
      text(tr('Каждое задание приносит {0}, все три — ещё 5 осколков и {1} за каждый день подряд (до семи). Новые задания — каждый день в полночь по Москве.', votesAmount(15), votesAmount(10))),
    ]),
    petWindow('/static/icons/pet/boss.svg', tr('Босс недели'), [
      text(tr('Босса бьют все питомцы сайта. Удар не стоит ничего, питомец получает опыт и отдыхает два часа; каждый удар — осколок. Кто побьёт босса, делят {0} по нанесённому урону, последний удар — ещё {1}. Новый босс — каждый понедельник.', votesAmount(boss.prize), votesAmount(boss.lastHit))),
    ]),
    petWindow('/static/icons/pet/cup.svg', tr('Турнир выходного дня'), [
      text(tr('До шестнадцати питомцев, по одному от человека, бьются на выбывание в воскресенье в 20:00 по Москве. Питомцы в турнире не устают и не теряют рейтинг. Победителю — {0}, финалисту — {1}, полуфиналистам — по {2}. Зрители ставят на питомцев: поставившие на победителя делят все ставки, кроме 10%.', votesAmount(1000), votesAmount(400), votesAmount(150))),
    ]),
    petWindow('/static/icons/pet/season.svg', tr('Сезон'), [
      text(tr('Сезон длится месяц и кончается {0}. Десятка лучших по рейтингу (по одному питомцу от человека) получает от {1} до {2}, затем рейтинг всех питомцев вдвое приближается к начальному.', formatDateTime(arena.seasonEndsAt), votesAmount(200), votesAmount(2000))),
    ]),
    petWindow('/static/icons/pet/market.svg', tr('Рынок питомцев'), [
      text(tr('Питомца можно продать другому человеку через неделю после того, как он у вас появился, и не дешевле {0} за обычного, {1} за редкого, {2} за эпического и {3} за легендарного. Казна берёт {4}% цены, до трёх питомцев на продаже сразу; выставленный питомец не дерётся.', groupedVotes(minPrices.common), groupedVotes(minPrices.rare), groupedVotes(minPrices.epic), groupedVotes(minPrices.legendary), marketFee)),
    ]),
    petWindow('/static/icons/pet/more.svg', tr('Породы'), [breeds(catalogue)]),
  ];
}
