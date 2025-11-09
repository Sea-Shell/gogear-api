BEGIN TRANSACTION;

-- Ensure users table matches new schema.
ALTER TABLE users ADD COLUMN userIsAdmin INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN userIsExternal INTEGER NOT NULL DEFAULT 0;

-- Ensure gear table has new sizing and container metadata.
ALTER TABLE gear ADD COLUMN gearIsContainer INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gear ADD COLUMN gearSizeDefinition TEXT;

-- Create table for registering contained gear.
CREATE TABLE IF NOT EXISTS user_container_registration (
    containerRegistrationId INTEGER PRIMARY KEY AUTOINCREMENT,
    userContainerId INTEGER NOT NULL,
    userGearRegistrationId INTEGER NOT NULL,
    FOREIGN KEY (userContainerId) REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE CASCADE,
    FOREIGN KEY (userGearRegistrationId) REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS user_container_registration_container_idx
    ON user_container_registration(userContainerId);
CREATE INDEX IF NOT EXISTS user_container_registration_gear_idx
    ON user_container_registration(userGearRegistrationId);

-- Update existing user records to populate the new flags.
UPDATE users
SET userIsAdmin = 1,
    userIsExternal = 1
WHERE userUsername = 'Bateau';

INSERT INTO users (userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal)
SELECT 'Mats', '', 'Mats Bøe Bergmann', 'mats@mm-ent.no', 0, 1
WHERE NOT EXISTS (SELECT 1 FROM users WHERE userUsername = 'Mats');

-- Mark existing backpack registrations as containers.
UPDATE gear
SET gearIsContainer = 1
WHERE gearIsContainer = 0
  AND gearCategoryId IN (
        SELECT gc.categoryId
        FROM gear_category AS gc
        JOIN gear_top_category AS gtc ON gc.categoryTopCategoryId = gtc.topCategoryId
        WHERE gtc.topCategoryName = 'Backpacks'
    );

-- Generate synthetic gear inventory while preventing duplicate names.
WITH RECURSIVE gear_items(n) AS (
    SELECT 1
    UNION ALL
    SELECT n + 1 FROM gear_items WHERE n < 100
),
adjective(idx, word) AS (
    VALUES
        (1, 'Summit'),
        (2, 'Cascade'),
        (3, 'Frontier'),
        (4, 'Granite'),
        (5, 'Highline'),
        (6, 'Aurora'),
        (7, 'Ember'),
        (8, 'Timberline'),
        (9, 'Wildland'),
        (10, 'Glacier'),
        (11, 'Stonepath'),
        (12, 'Windward'),
        (13, 'Ridgecrest'),
        (14, 'Trailbound'),
        (15, 'Brightstar'),
        (16, 'Northshore'),
        (17, 'Blue Ridge'),
        (18, 'Copperleaf'),
        (19, 'Starlight'),
        (20, 'Evervale')
),
terrain(idx, word) AS (
    VALUES
        (1, 'Explorer'),
        (2, 'Trail'),
        (3, 'Range'),
        (4, 'Expedition'),
        (5, 'Voyager'),
        (6, 'Highland'),
        (7, 'Traverse'),
        (8, 'Wilderness'),
        (9, 'Alpine'),
        (10, 'Coastal'),
        (11, 'Peak'),
        (12, 'Forest'),
        (13, 'Ridgeline'),
        (14, 'Backcountry'),
        (15, 'Scout'),
        (16, 'Nomad'),
        (17, 'Pathfinder'),
        (18, 'Field'),
        (19, 'Outpost'),
        (20, 'Expanse')
),
descriptor(idx, word) AS (
    VALUES
        (1, 'Pro'),
        (2, 'Elite'),
        (3, 'Classic'),
        (4, 'Lite'),
        (5, 'Max'),
        (6, 'Ultra'),
        (7, 'Heritage'),
        (8, 'Prime'),
        (9, 'Advance'),
        (10, 'Performance'),
        (11, 'Motion'),
        (12, 'Shield'),
        (13, 'Forge'),
        (14, 'Quest'),
        (15, 'Core'),
        (16, 'Venture'),
        (17, 'Flex'),
        (18, 'Edge'),
        (19, 'Signature'),
        (20, 'Edition')
),
category_label AS (
    SELECT
        categoryId,
        CASE categoryName
            WHEN 'Hiking boots' THEN 'Hiking Boots'
            WHEN 'Trail shoes' THEN 'Trail Shoes'
            WHEN 'Hiking socks' THEN 'Hiking Socks'
            WHEN 'Moisture-wicking base layers' THEN 'Moisture-Wicking Base Layers'
            WHEN 'Moisture-wicking shirts' THEN 'Moisture-Wicking Shirts'
            WHEN 'Hiking pants' THEN 'Hiking Pants'
            WHEN 'shorts' THEN 'Shorts'
            WHEN 'Rain jacket' THEN 'Rain Jacket'
            WHEN 'Rain pants' THEN 'Rain Pants'
            WHEN 'Sun hat' THEN 'Sun Hat'
            WHEN 'Hiking backpack' THEN 'Hiking Backpack'
            WHEN 'Hydration backpack' THEN 'Hydration Backpack'
            WHEN 'First aid kit' THEN 'First Aid Kit'
            WHEN 'Multi-tool' THEN 'Multi-Tool'
            WHEN 'knife' THEN 'Knife'
            WHEN 'Emergency space blanket' THEN 'Emergency Space Blanket'
            WHEN 'Bivy sack' THEN 'Bivy Sack'
            WHEN 'Ground tarp' THEN 'Ground Tarp'
            WHEN 'Sleeping bag' THEN 'Sleeping Bag'
            WHEN 'Sleeping pad' THEN 'Sleeping Pad'
            WHEN 'Air mattress' THEN 'Air Mattress'
            WHEN 'Lightweight food' THEN 'Lightweight Food'
            WHEN 'Water bottle' THEN 'Water Bottle'
            WHEN 'Water purification system' THEN 'Water Purification System'
            WHEN 'Bear canister' THEN 'Bear Canister'
            WHEN 'Trekking poles' THEN 'Trekking Poles'
            WHEN 'Insect repellent' THEN 'Insect Repellent'
            WHEN 'Satellite communicator' THEN 'Satellite Communicator'
            WHEN 'Two-way radios' THEN 'Two-Way Radios'
            WHEN 'Neck gaiter' THEN 'Neck Gaiter'
            WHEN 'gloves' THEN 'Gloves'
            ELSE CASE
                WHEN categoryName LIKE '% %' THEN
                    TRIM(
                        REPLACE(
                            REPLACE(
                                REPLACE(
                                    REPLACE(
                                        REPLACE(categoryName,
                                            ' backpack', ' Backpack'),
                                        ' jacket', ' Jacket'),
                                    ' pants', ' Pants'),
                                ' hat', ' Hat'),
                            ' kit', ' Kit')
                    )
                ELSE UPPER(SUBSTR(categoryName, 1, 1)) || SUBSTR(categoryName, 2)
            END
        END AS label
    FROM gear_category
),
counts AS (
    SELECT
        (SELECT COUNT(*) FROM adjective) AS adjective_total,
        (SELECT COUNT(*) FROM terrain) AS terrain_total,
        (SELECT COUNT(*) FROM descriptor) AS descriptor_total
),
backpack_top AS (
    SELECT topCategoryId AS id FROM gear_top_category WHERE topCategoryName = 'Backpacks' LIMIT 1
),
generated_gear AS (
    SELECT
        gc.categoryTopCategoryId AS gearTopCategoryId,
        gc.categoryId AS gearCategoryId,
        (( (gc.categoryId - 1) * 100 + gear_items.n - 1) % (SELECT COUNT(*) FROM manufacture)) + 1 AS gearManufactureId,
        CASE WHEN gc.categoryTopCategoryId = bt.id THEN 1 ELSE 0 END AS gearIsContainer,
        adj.word || ' ' || terrain.word || ' ' || descriptor.word || ' ' || cl.label AS gearName,
        CASE ((gear_items.n - 1) % 3)
            WHEN 0 THEN 'Size S'
            WHEN 1 THEN 'Size M'
            ELSE 'Size L'
        END AS gearSizeDefinition,
        250 + ((gear_items.n - 1) % 50) * 15 + ((gc.categoryId - 1) % 12) AS gearWeight,
        40 + ((gear_items.n - 1) % 60) AS gearHeight,
        90 + ((gc.categoryId - 1) % 40) AS gearLength,
        30 + ((gear_items.n - 1) % 25) AS gearWidth,
        CASE WHEN (gear_items.n % 11) = 0 THEN 0 ELSE 1 END AS gearStatus
    FROM gear_category AS gc
    CROSS JOIN gear_items
    CROSS JOIN counts
    JOIN backpack_top AS bt
    JOIN adjective AS adj ON adj.idx = (( (gc.categoryId - 1) * 100 + gear_items.n - 1) % counts.adjective_total) + 1
    JOIN terrain ON terrain.idx = (( (gear_items.n - 1) + (gc.categoryTopCategoryId - 1) * 7) % counts.terrain_total) + 1
    JOIN descriptor ON descriptor.idx = (( (gc.categoryId - 1) * 37 + gear_items.n - 1) % counts.descriptor_total) + 1
    JOIN category_label AS cl ON cl.categoryId = gc.categoryId
)
INSERT INTO gear (
    gearTopCategoryId,
    gearCategoryId,
    gearManufactureId,
    gearIsContainer,
    gearName,
    gearSizeDefinition,
    gearWeight,
    gearHeight,
    gearLength,
    gearWidth,
    gearStatus
)
SELECT
    gg.gearTopCategoryId,
    gg.gearCategoryId,
    gg.gearManufactureId,
    gg.gearIsContainer,
    gg.gearName,
    gg.gearSizeDefinition,
    gg.gearWeight,
    gg.gearHeight,
    gg.gearLength,
    gg.gearWidth,
    gg.gearStatus
FROM generated_gear AS gg
WHERE NOT EXISTS (
    SELECT 1
    FROM gear AS existing
    WHERE existing.gearName = gg.gearName
);

COMMIT;
