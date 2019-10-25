use `isucari`;

ALTER TABLE `transaction_evidences`
    DROP COLUMN `item_name`,
    -- DROP COLUMN `item_price`,
    DROP COLUMN `item_description`,
    DROP COLUMN `item_category_id`,
    DROP COLUMN `item_root_category_id`;

ALTER TABLE `shippings`
    DROP COLUMN `item_name`,
    DROP COLUMN `reserve_time`,
    DROP COLUMN `to_address`,
    DROP COLUMN `to_name`,
    DROP COLUMN `from_address`,
    DROP COLUMN `from_name`;
