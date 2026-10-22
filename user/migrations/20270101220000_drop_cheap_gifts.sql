-- +goose Up
-- The flowers, the sweets and the holidays are no longer in the catalogue:
-- the gifts of them that were sent go, and so do their prices. Their
-- notifications stay in notification, which knows no kind of gift; they
-- lead to the recipient's gifts.
DELETE FROM gifts
WHERE gift IN (
    'rose', 'tulip', 'bouquet', 'sunflower', 'cactus',
    'cake', 'candy', 'chocolate', 'icecream', 'cupcake',
    'present', 'balloons', 'heart', 'bear', 'star'
);

DELETE FROM gift_prices
WHERE gift IN (
    'rose', 'tulip', 'bouquet', 'sunflower', 'cactus',
    'cake', 'candy', 'chocolate', 'icecream', 'cupcake',
    'present', 'balloons', 'heart', 'bear', 'star'
);

-- +goose Down
-- The gifts are gone for good; there is nothing to bring back.
SELECT 1;
