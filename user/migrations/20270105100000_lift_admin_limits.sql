-- +goose Up
-- The numbers the administrators enter have no upper bounds but those of
-- the arithmetic (user.MaxAmount, user.MaxMultiplier): the treasury has no
-- bottom, so the casino, the prices and the stakes need no ceiling.
ALTER TABLE vote_rates
    DROP CONSTRAINT vote_rates_kopecks_check,
    ADD CONSTRAINT vote_rates_kopecks_check CHECK (kopecks >= 1);

ALTER TABLE gift_prices
    DROP CONSTRAINT gift_prices_price_check,
    ADD CONSTRAINT gift_prices_price_check CHECK (price >= 0);

ALTER TABLE custom_sticker_packs
    DROP CONSTRAINT custom_sticker_packs_price_check,
    ADD CONSTRAINT custom_sticker_packs_price_check CHECK (price >= 0);

ALTER TABLE casino
    DROP CONSTRAINT casino_price_check,
    ADD CONSTRAINT casino_price_check CHECK (price >= 1),
    DROP CONSTRAINT casino_bet_limits,
    ADD CONSTRAINT casino_bet_limits CHECK (bet_least BETWEEN 1 AND bet_most),
    DROP CONSTRAINT casino_rocket_limits,
    ADD CONSTRAINT casino_rocket_limits CHECK (rocket_least BETWEEN 1 AND rocket_most),
    DROP CONSTRAINT casino_jackpot_check,
    ADD CONSTRAINT casino_jackpot_check CHECK (jackpot >= 1);

ALTER TABLE bet_events
    DROP CONSTRAINT bet_events_check1,
    ADD CONSTRAINT bet_events_odds_check CHECK (
        odds IS NULL
        OR (cardinality(odds) = cardinality(outcomes) AND 101 <= ALL (odds) AND 1000000 >= ALL (odds))
    );

ALTER TABLE prime_prices
    DROP CONSTRAINT prime_prices_month_check,
    ADD CONSTRAINT prime_prices_month_check CHECK (month >= 1),
    DROP CONSTRAINT prime_prices_quarter_check,
    ADD CONSTRAINT prime_prices_quarter_check CHECK (quarter >= 1),
    DROP CONSTRAINT prime_prices_year_check,
    ADD CONSTRAINT prime_prices_year_check CHECK (year >= 1);

ALTER TABLE gifts
    DROP CONSTRAINT gifts_sale_price_check,
    ADD CONSTRAINT gifts_sale_price_check CHECK (sale_price >= 1);

ALTER TABLE gift_stock
    DROP CONSTRAINT gift_stock_supply_check,
    ADD CONSTRAINT gift_stock_supply_check CHECK (supply >= 0);

ALTER TABLE gift_auctions
    DROP CONSTRAINT gift_auctions_start_price_check,
    ADD CONSTRAINT gift_auctions_start_price_check CHECK (start_price >= 1),
    DROP CONSTRAINT gift_auctions_top_bid_check,
    ADD CONSTRAINT gift_auctions_top_bid_check CHECK (top_bid >= 0);

-- The economy of the site, set by the administrators: the daily bonus and
-- the fees in percent — of a transfer, of the sale of a sticker pack, and
-- of the same with Prime. No row — user.DefaultEconomy.
CREATE TABLE economy (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    bonus bigint NOT NULL CHECK (bonus >= 1),
    transfer_fee integer NOT NULL CHECK (transfer_fee BETWEEN 0 AND 100),
    sticker_tax integer NOT NULL CHECK (sticker_tax BETWEEN 0 AND 100),
    prime_sticker_tax integer NOT NULL CHECK (prime_sticker_tax BETWEEN 0 AND 100)
);

-- +goose Down
DROP TABLE economy;

-- The old bounds come back NOT VALID: values set since stay as they are.
ALTER TABLE gift_auctions
    DROP CONSTRAINT gift_auctions_top_bid_check,
    ADD CONSTRAINT gift_auctions_top_bid_check CHECK (top_bid BETWEEN 0 AND 1000000) NOT VALID,
    DROP CONSTRAINT gift_auctions_start_price_check,
    ADD CONSTRAINT gift_auctions_start_price_check CHECK (start_price BETWEEN 1 AND 1000000) NOT VALID;

ALTER TABLE gift_stock
    DROP CONSTRAINT gift_stock_supply_check,
    ADD CONSTRAINT gift_stock_supply_check CHECK (supply BETWEEN 0 AND 1000000) NOT VALID;

ALTER TABLE gifts
    DROP CONSTRAINT gifts_sale_price_check,
    ADD CONSTRAINT gifts_sale_price_check CHECK (sale_price BETWEEN 1 AND 1000000) NOT VALID;

ALTER TABLE prime_prices
    DROP CONSTRAINT prime_prices_year_check,
    ADD CONSTRAINT prime_prices_year_check CHECK (year BETWEEN 1 AND 1000000) NOT VALID,
    DROP CONSTRAINT prime_prices_quarter_check,
    ADD CONSTRAINT prime_prices_quarter_check CHECK (quarter BETWEEN 1 AND 1000000) NOT VALID,
    DROP CONSTRAINT prime_prices_month_check,
    ADD CONSTRAINT prime_prices_month_check CHECK (month BETWEEN 1 AND 1000000) NOT VALID;

ALTER TABLE bet_events
    DROP CONSTRAINT bet_events_odds_check,
    ADD CONSTRAINT bet_events_check1 CHECK (
        odds IS NULL
        OR (cardinality(odds) = cardinality(outcomes) AND 101 <= ALL (odds) AND 100000 >= ALL (odds))
    ) NOT VALID;

ALTER TABLE casino
    DROP CONSTRAINT casino_jackpot_check,
    ADD CONSTRAINT casino_jackpot_check CHECK (jackpot BETWEEN 1 AND 1000) NOT VALID,
    DROP CONSTRAINT casino_rocket_limits,
    ADD CONSTRAINT casino_rocket_limits CHECK (rocket_least BETWEEN 1 AND rocket_most AND rocket_most <= 100000000) NOT VALID,
    DROP CONSTRAINT casino_bet_limits,
    ADD CONSTRAINT casino_bet_limits CHECK (bet_least BETWEEN 1 AND bet_most AND bet_most <= 100000000) NOT VALID,
    DROP CONSTRAINT casino_price_check,
    ADD CONSTRAINT casino_price_check CHECK (price BETWEEN 1 AND 10000) NOT VALID;

ALTER TABLE custom_sticker_packs
    DROP CONSTRAINT custom_sticker_packs_price_check,
    ADD CONSTRAINT custom_sticker_packs_price_check CHECK (price BETWEEN 0 AND 1000) NOT VALID;

ALTER TABLE gift_prices
    DROP CONSTRAINT gift_prices_price_check,
    ADD CONSTRAINT gift_prices_price_check CHECK (price BETWEEN 0 AND 1000000) NOT VALID;

ALTER TABLE vote_rates
    DROP CONSTRAINT vote_rates_kopecks_check,
    ADD CONSTRAINT vote_rates_kopecks_check CHECK (kopecks BETWEEN 1 AND 100000000) NOT VALID;
