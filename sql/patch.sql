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
    FOREIGN KEY (userContainerId) REFERENCES user_gear_registrations(userGearRegistrationId),
    FOREIGN KEY (userGearRegistrationId) REFERENCES user_gear_registrations(userGearRegistrationId)
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

-- Seed top-level gear categories without duplicating existing rows.
WITH new_top(topCategoryName) AS (
    VALUES
        ('Footwear'),
        ('Clothing'),
        ('Backpacks'),
        ('Navigation and Safety'),
        ('Shelter'),
        ('Sleeping Gear'),
        ('Cooking'),
        ('Hiking Accessories'),
        ('Emergency and Communication'),
        ('Apparel Accessories')
)
INSERT INTO gear_top_category (topCategoryName)
SELECT topCategoryName
FROM new_top
WHERE NOT EXISTS (
    SELECT 1
    FROM gear_top_category existing
    WHERE existing.topCategoryName = new_top.topCategoryName
);

-- Seed gear categories tied to their parent top categories.
WITH new_categories(categoryName, topCategoryName) AS (
    VALUES
        ('Hiking boots', 'Footwear'),
        ('Trail shoes', 'Footwear'),
        ('Hiking socks', 'Footwear'),
        ('Gaiters', 'Footwear'),
        ('Moisture-wicking base layers', 'Clothing'),
        ('Moisture-wicking shirts', 'Clothing'),
        ('Hiking pants', 'Clothing'),
        ('shorts', 'Clothing'),
        ('Insulation', 'Clothing'),
        ('Rain jacket', 'Clothing'),
        ('Rain pants', 'Clothing'),
        ('Hat', 'Clothing'),
        ('Sun hat', 'Clothing'),
        ('Beanie', 'Clothing'),
        ('Gloves', 'Clothing'),
        ('Hiking backpack', 'Backpacks'),
        ('Daypack', 'Backpacks'),
        ('Hydration backpack', 'Backpacks'),
        ('Maps', 'Navigation and Safety'),
        ('Compass', 'Navigation and Safety'),
        ('GPS', 'Navigation and Safety'),
        ('Smartphone', 'Navigation and Safety'),
        ('Whistle', 'Navigation and Safety'),
        ('First aid kit', 'Navigation and Safety'),
        ('Multi-tool', 'Navigation and Safety'),
        ('knife', 'Navigation and Safety'),
        ('Tent', 'Shelter'),
        ('Footprint', 'Shelter'),
        ('Tarp', 'Shelter'),
        ('Emergency space blanket', 'Shelter'),
        ('Bivy sack', 'Shelter'),
        ('Ground tarp', 'Shelter'),
        ('Sleeping bag', 'Sleeping Gear'),
        ('Duvet', 'Sleeping Gear'),
        ('Sleeping pad', 'Sleeping Gear'),
        ('Air mattress', 'Sleeping Gear'),
        ('Pillow', 'Sleeping Gear'),
        ('Stove', 'Cooking'),
        ('Fuel', 'Cooking'),
        ('Cookware Pot', 'Cooking'),
        ('Pan', 'Cooking'),
        ('Utensils', 'Cooking'),
        ('Lightweight food', 'Cooking'),
        ('Water bottle', 'Cooking'),
        ('Water purification system', 'Cooking'),
        ('Bear canister', 'Hiking Accessories'),
        ('Trekking poles', 'Hiking Accessories'),
        ('Sunglasses', 'Hiking Accessories'),
        ('Sunscreen', 'Hiking Accessories'),
        ('Insect repellent', 'Hiking Accessories'),
        ('Headlamp', 'Hiking Accessories'),
        ('Flashlight', 'Hiking Accessories'),
        ('Batteries', 'Hiking Accessories'),
        ('Camera', 'Hiking Accessories'),
        ('Satellite communicator', 'Emergency and Communication'),
        ('Personal Locator Beacon', 'Emergency and Communication'),
        ('Two-way radios', 'Emergency and Communication'),
        ('Bandana', 'Apparel Accessories'),
        ('Buff', 'Apparel Accessories'),
        ('Neck gaiter', 'Apparel Accessories'),
        ('gloves', 'Apparel Accessories')
)
INSERT INTO gear_category (categoryName, categoryTopCategoryId)
SELECT nc.categoryName, gtc.topCategoryId
FROM new_categories AS nc
JOIN gear_top_category AS gtc ON gtc.topCategoryName = nc.topCategoryName
WHERE NOT EXISTS (
    SELECT 1
    FROM gear_category existing
    WHERE existing.categoryName = nc.categoryName
      AND existing.categoryTopCategoryId = gtc.topCategoryId
);

-- Seed manufacturer catalog and keep names in sync.
WITH new_manufacture(manufactureId, manufactureName) AS (
    VALUES
        (1, 'The North Face'),
        (2, 'Patagonia'),
        (3, 'Columbia Sportswear'),
        (4, 'Arc''teryx'),
        (5, 'Salomon'),
        (6, 'Outdoor Research'),
        (7, 'Marmot'),
        (8, 'Black Diamond Equipment'),
        (9, 'Osprey'),
        (10, 'Gregory'),
        (11, 'Deuter'),
        (12, 'Kelty'),
        (13, 'MSR (Mountain Safety Research)'),
        (14, 'Petzl'),
        (15, 'Merrell'),
        (16, 'Keen'),
        (17, 'Vasque'),
        (18, 'La Sportiva'),
        (19, 'Scarpa'),
        (20, 'Mammut'),
        (21, 'Hilleberg'),
        (22, 'Big Agnes'),
        (23, 'Therm-a-Rest'),
        (24, 'REI Co-op'),
        (25, 'Gossamer Gear'),
        (26, 'Granite Gear'),
        (27, 'Sea to Summit'),
        (28, 'Rab'),
        (29, 'Montane'),
        (30, 'Fjällräven'),
        (31, 'Hoka One One'),
        (32, 'Oboz'),
        (33, 'Altra'),
        (34, 'Inov-8'),
        (35, 'Lowa'),
        (36, 'Exped'),
        (37, 'Hyperlite Mountain Gear'),
        (38, 'NEMO Equipment'),
        (39, 'Western Mountaineering'),
        (40, 'MontBell'),
        (41, 'Garmont'),
        (42, 'Salewa'),
        (43, 'ORTOVOX'),
        (44, 'Snow Peak'),
        (45, 'Cotopaxi'),
        (46, 'Klymit'),
        (47, 'Blackyak'),
        (48, 'Zamberlan'),
        (49, 'Norrøna'),
        (50, 'Devold'),
        (51, 'Sweet Protection'),
        (52, 'Lundhags'),
        (53, 'Haglöfs'),
        (54, 'Millet'),
        (55, 'Vaude'),
        (56, 'Wild Country'),
        (57, 'Grivel'),
        (58, 'CAMP'),
        (59, 'Edelrid'),
        (60, 'Sterling Rope'),
        (61, 'BlueWater Ropes'),
        (62, 'Five Ten'),
        (63, 'Evolv'),
        (64, 'Metolius Climbing'),
        (65, 'Beal'),
        (66, 'Maxim Ropes'),
        (67, 'Trango'),
        (68, 'Edelweiss'),
        (69, 'Misty Mountain'),
        (70, 'Camp USA'),
        (71, 'Cassin'),
        (72, 'DMM'),
        (73, 'Houdini'),
        (74, 'Didriksons'),
        (75, 'Helly Hansen'),
        (76, 'Bach'),
        (77, 'Peak Performance'),
        (78, 'Arctix'),
        (79, 'Ulvang'),
        (80, '66°North'),
        (81, 'Hestra'),
        (82, 'Bula'),
        (83, 'O''Neill'),
        (84, 'Kari Traa'),
        (85, 'Dale of Norway'),
        (86, 'Icebreaker'),
        (87, 'Trangia'),
        (88, 'Bergans'),
        (89, 'Crispi'),
        (90, 'Summit Forge'),
        (91, 'Trailblazer Works'),
        (92, 'PeakLine Outfitters'),
        (93, 'Northbound Gear'),
        (94, 'Evercrest Equipment'),
        (95, 'Red Ridge Supply'),
        (96, 'OpenSky Outfitters'),
        (97, 'Stonepath Gear'),
        (98, 'Glacier Trail Co.'),
        (99, 'Wild Horizon'),
        (100, 'Alpine Lantern'),
        (101, 'Summit Stitch'),
        (102, 'Outrider Gear'),
        (103, 'Aurora Fieldworks'),
        (104, 'Bright Peak Supply'),
        (105, 'Highline Provisions'),
        (106, 'Stonepine Outfitters'),
        (107, 'Ironwood Gear'),
        (108, 'Cinder Trail Company'),
        (109, 'Emberlight Labs'),
        (110, 'Cascade Workshop'),
        (111, 'SummitCircle'),
        (112, 'Trailstone Collective'),
        (113, 'Wanderforge'),
        (114, 'Blue Spur Gear'),
        (115, 'Lumen Ridge'),
        (116, 'Frostline Outfitters'),
        (117, 'Granite Lantern'),
        (118, 'Cobalt Peak'),
        (119, 'Timberline & Co.'),
        (120, 'Ridgecrest Outfitters'),
        (121, 'Starfall Gear'),
        (122, 'Northwind Supply'),
        (123, 'Emberfall Works'),
        (124, 'Summit Compass'),
        (125, 'Traillight Equipment'),
        (126, 'Foxpine Gear'),
        (127, 'Lone Summit Outfitters'),
        (128, 'Brightstone Gear'),
        (129, 'Pioneer Ridge'),
        (130, 'Coppertrail'),
        (131, 'Silver Fir'),
        (132, 'Nomad Forge'),
        (133, 'Emberwild'),
        (134, 'Fjordstone'),
        (135, 'Tidecrest'),
        (136, 'Summit Ember'),
        (137, 'Riverlight'),
        (138, 'Highland Axis'),
        (139, 'Arctic Beacon'),
        (140, 'Pine & Peak'),
        (141, 'Trail & Timber'),
        (142, 'Stellarsky Outfitters'),
        (143, 'Horizon Ridge'),
        (144, 'Cairnline Gear'),
        (145, 'Peak Junction'),
        (146, 'Cloudveil Works'),
        (147, 'Summitstone Outfitters'),
        (148, 'Wildfell Gear'),
        (149, 'Northbound Atelier'),
        (150, 'Everpine Supply'),
        (151, 'Beaconrise'),
        (152, 'Snowforge'),
        (153, 'Highpoint Outfitters'),
        (154, 'Trailcrest Studio'),
        (155, 'Granite Loom'),
        (156, 'Wilderline'),
        (157, 'Moonridge'),
        (158, 'Lodestone Gear'),
        (159, 'Red Ember Outfitters'),
        (160, 'Crosswind Equipment'),
        (161, 'Fieldwake'),
        (162, 'Helios Trail'),
        (163, 'Ironcrest'),
        (164, 'Northspur'),
        (165, 'Quarrylight'),
        (166, 'Silver Timber Gear'),
        (167, 'Summit Loom'),
        (168, 'Tundra Echo'),
        (169, 'Wildspire'),
        (170, 'Alpenglow Forge'),
        (171, 'Boreal Crest'),
        (172, 'Canyonline'),
        (173, 'Driftstone'),
        (174, 'Echo Ridge'),
        (175, 'Foxfire Outfitters'),
        (176, 'Glint Peak'),
        (177, 'High Fjord Gear'),
        (178, 'Icetrail'),
        (179, 'Jasper Summit'),
        (180, 'Kestrel Ridge'),
        (181, 'Lumen Forge'),
        (182, 'Mistral Equipment'),
        (183, 'Northcairn'),
        (184, 'Open Range Gear'),
        (185, 'Pineforge'),
        (186, 'Quartzline'),
        (187, 'Ridgefire'),
        (188, 'Stonehollow'),
        (189, 'Timbercrest'),
        (190, 'Ultralight Labs'),
        (191, 'Valleyforge'),
        (192, 'Windward Gear'),
        (193, 'Xenith Outfitters'),
        (194, 'Yellowstone Works'),
        (195, 'Zephyr Trail'),
        (196, 'Amber Summit'),
        (197, 'Bearcrest Gear'),
        (198, 'Canyon Forge'),
        (199, 'Denali Outfitters'),
        (200, 'Embercrest Supply')
)
INSERT OR IGNORE INTO manufacture (manufactureId, manufactureName)
SELECT manufactureId, manufactureName
FROM new_manufacture;

WITH new_manufacture(manufactureId, manufactureName) AS (
    VALUES
        (1, 'The North Face'),
        (2, 'Patagonia'),
        (3, 'Columbia Sportswear'),
        (4, 'Arc''teryx'),
        (5, 'Salomon'),
        (6, 'Outdoor Research'),
        (7, 'Marmot'),
        (8, 'Black Diamond Equipment'),
        (9, 'Osprey'),
        (10, 'Gregory'),
        (11, 'Deuter'),
        (12, 'Kelty'),
        (13, 'MSR (Mountain Safety Research)'),
        (14, 'Petzl'),
        (15, 'Merrell'),
        (16, 'Keen'),
        (17, 'Vasque'),
        (18, 'La Sportiva'),
        (19, 'Scarpa'),
        (20, 'Mammut'),
        (21, 'Hilleberg'),
        (22, 'Big Agnes'),
        (23, 'Therm-a-Rest'),
        (24, 'REI Co-op'),
        (25, 'Gossamer Gear'),
        (26, 'Granite Gear'),
        (27, 'Sea to Summit'),
        (28, 'Rab'),
        (29, 'Montane'),
        (30, 'Fjällräven'),
        (31, 'Hoka One One'),
        (32, 'Oboz'),
        (33, 'Altra'),
        (34, 'Inov-8'),
        (35, 'Lowa'),
        (36, 'Exped'),
        (37, 'Hyperlite Mountain Gear'),
        (38, 'NEMO Equipment'),
        (39, 'Western Mountaineering'),
        (40, 'MontBell'),
        (41, 'Garmont'),
        (42, 'Salewa'),
        (43, 'ORTOVOX'),
        (44, 'Snow Peak'),
        (45, 'Cotopaxi'),
        (46, 'Klymit'),
        (47, 'Blackyak'),
        (48, 'Zamberlan'),
        (49, 'Norrøna'),
        (50, 'Devold'),
        (51, 'Sweet Protection'),
        (52, 'Lundhags'),
        (53, 'Haglöfs'),
        (54, 'Millet'),
        (55, 'Vaude'),
        (56, 'Wild Country'),
        (57, 'Grivel'),
        (58, 'CAMP'),
        (59, 'Edelrid'),
        (60, 'Sterling Rope'),
        (61, 'BlueWater Ropes'),
        (62, 'Five Ten'),
        (63, 'Evolv'),
        (64, 'Metolius Climbing'),
        (65, 'Beal'),
        (66, 'Maxim Ropes'),
        (67, 'Trango'),
        (68, 'Edelweiss'),
        (69, 'Misty Mountain'),
        (70, 'Camp USA'),
        (71, 'Cassin'),
        (72, 'DMM'),
        (73, 'Houdini'),
        (74, 'Didriksons'),
        (75, 'Helly Hansen'),
        (76, 'Bach'),
        (77, 'Peak Performance'),
        (78, 'Arctix'),
        (79, 'Ulvang'),
        (80, '66°North'),
        (81, 'Hestra'),
        (82, 'Bula'),
        (83, 'O''Neill'),
        (84, 'Kari Traa'),
        (85, 'Dale of Norway'),
        (86, 'Icebreaker'),
        (87, 'Trangia'),
        (88, 'Bergans'),
        (89, 'Crispi'),
        (90, 'Summit Forge'),
        (91, 'Trailblazer Works'),
        (92, 'PeakLine Outfitters'),
        (93, 'Northbound Gear'),
        (94, 'Evercrest Equipment'),
        (95, 'Red Ridge Supply'),
        (96, 'OpenSky Outfitters'),
        (97, 'Stonepath Gear'),
        (98, 'Glacier Trail Co.'),
        (99, 'Wild Horizon'),
        (100, 'Alpine Lantern'),
        (101, 'Summit Stitch'),
        (102, 'Outrider Gear'),
        (103, 'Aurora Fieldworks'),
        (104, 'Bright Peak Supply'),
        (105, 'Highline Provisions'),
        (106, 'Stonepine Outfitters'),
        (107, 'Ironwood Gear'),
        (108, 'Cinder Trail Company'),
        (109, 'Emberlight Labs'),
        (110, 'Cascade Workshop'),
        (111, 'SummitCircle'),
        (112, 'Trailstone Collective'),
        (113, 'Wanderforge'),
        (114, 'Blue Spur Gear'),
        (115, 'Lumen Ridge'),
        (116, 'Frostline Outfitters'),
        (117, 'Granite Lantern'),
        (118, 'Cobalt Peak'),
        (119, 'Timberline & Co.'),
        (120, 'Ridgecrest Outfitters'),
        (121, 'Starfall Gear'),
        (122, 'Northwind Supply'),
        (123, 'Emberfall Works'),
        (124, 'Summit Compass'),
        (125, 'Traillight Equipment'),
        (126, 'Foxpine Gear'),
        (127, 'Lone Summit Outfitters'),
        (128, 'Brightstone Gear'),
        (129, 'Pioneer Ridge'),
        (130, 'Coppertrail'),
        (131, 'Silver Fir'),
        (132, 'Nomad Forge'),
        (133, 'Emberwild'),
        (134, 'Fjordstone'),
        (135, 'Tidecrest'),
        (136, 'Summit Ember'),
        (137, 'Riverlight'),
        (138, 'Highland Axis'),
        (139, 'Arctic Beacon'),
        (140, 'Pine & Peak'),
        (141, 'Trail & Timber'),
        (142, 'Stellarsky Outfitters'),
        (143, 'Horizon Ridge'),
        (144, 'Cairnline Gear'),
        (145, 'Peak Junction'),
        (146, 'Cloudveil Works'),
        (147, 'Summitstone Outfitters'),
        (148, 'Wildfell Gear'),
        (149, 'Northbound Atelier'),
        (150, 'Everpine Supply'),
        (151, 'Beaconrise'),
        (152, 'Snowforge'),
        (153, 'Highpoint Outfitters'),
        (154, 'Trailcrest Studio'),
        (155, 'Granite Loom'),
        (156, 'Wilderline'),
        (157, 'Moonridge'),
        (158, 'Lodestone Gear'),
        (159, 'Red Ember Outfitters'),
        (160, 'Crosswind Equipment'),
        (161, 'Fieldwake'),
        (162, 'Helios Trail'),
        (163, 'Ironcrest'),
        (164, 'Northspur'),
        (165, 'Quarrylight'),
        (166, 'Silver Timber Gear'),
        (167, 'Summit Loom'),
        (168, 'Tundra Echo'),
        (169, 'Wildspire'),
        (170, 'Alpenglow Forge'),
        (171, 'Boreal Crest'),
        (172, 'Canyonline'),
        (173, 'Driftstone'),
        (174, 'Echo Ridge'),
        (175, 'Foxfire Outfitters'),
        (176, 'Glint Peak'),
        (177, 'High Fjord Gear'),
        (178, 'Icetrail'),
        (179, 'Jasper Summit'),
        (180, 'Kestrel Ridge'),
        (181, 'Lumen Forge'),
        (182, 'Mistral Equipment'),
        (183, 'Northcairn'),
        (184, 'Open Range Gear'),
        (185, 'Pineforge'),
        (186, 'Quartzline'),
        (187, 'Ridgefire'),
        (188, 'Stonehollow'),
        (189, 'Timbercrest'),
        (190, 'Ultralight Labs'),
        (191, 'Valleyforge'),
        (192, 'Windward Gear'),
        (193, 'Xenith Outfitters'),
        (194, 'Yellowstone Works'),
        (195, 'Zephyr Trail'),
        (196, 'Amber Summit'),
        (197, 'Bearcrest Gear'),
        (198, 'Canyon Forge'),
        (199, 'Denali Outfitters'),
        (200, 'Embercrest Supply')
)
UPDATE manufacture
SET manufactureName = (
    SELECT nm.manufactureName
    FROM new_manufacture AS nm
    WHERE nm.manufactureId = manufacture.manufactureId
)
WHERE EXISTS (
    SELECT 1
    FROM new_manufacture AS nm
    WHERE nm.manufactureId = manufacture.manufactureId
      AND nm.manufactureName <> manufacture.manufactureName
);

-- Ensure autoincrement counters stay in sync with seeded data.
UPDATE sqlite_sequence
SET seq = (
        SELECT MAX(manufactureId)
        FROM manufacture
    )
WHERE name = 'manufacture'
  AND seq < (
        SELECT MAX(manufactureId)
        FROM manufacture
    );

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
