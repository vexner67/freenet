-- +goose Up
-- The owner took away the site's own packs of stickers: nobody keeps the
-- smiles, the cats and the bears any more. Messages that sent one of them
-- keep its name and show "Стикер".
DELETE FROM sticker_packs WHERE pack IN ('smiles', 'cats', 'bears');

-- +goose Down
-- The packs taken away are not given back.
SELECT 1;
