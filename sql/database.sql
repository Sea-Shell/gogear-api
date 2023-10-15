CREATE TABLE IF NOT EXISTS users (
    userId INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    userUsername TEXT NOT NULL,
    userPassword TEXT NOT NULL,
    userName TEXT,
    userEmail TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS gear (
    gearId INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    gearTopCategoryId INTEGER NOT NULL,
    gearCategoryId INTEGER NOT NULL,
    gearManufactureId INTEGER NOT NULL,
    gearName TEXT NOT NULL,
    gearWeight INTEGER,
    gearHeight INTEGER,
    gearLength INTEGER,
    gearWidth INTEGER,
    gearStatus BOOLEAN,
    FOREIGN KEY (gearTopCategoryId) REFERENCES gear_top_category(topCategoryId),
    FOREIGN KEY (gearCategoryId) REFERENCES gear_category(categoryId),
    FOREIGN KEY (gearManufactureId) REFERENCES manufacture(manufactureId)
);
CREATE INDEX topCategory_index ON gear(gearTopCategoryId);
CREATE INDEX category_index ON gear(gearCategoryId);
CREATE INDEX manufacture_index ON gear(gearManufactureId);

CREATE TABLE IF NOT EXISTS manufacture (
    manufactureId INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    manufactureName TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS gear_top_category (
    topCategoryId INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    topCategoryName TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS gear_category (
    categoryId INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    categoryTopCategoryId INTEGER NOT NULL,
    categoryName TEXT NOT NULL,
    FOREIGN KEY (categoryTopCategoryId) REFERENCES gear_top_category(topCategoryId)
);

CREATE TABLE IF NOT EXISTS user_gear_registrations (
    userGearRegistrationId INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    gearId INTEGER NOT NULL,
    userId INTEGER NOT NULL,
    FOREIGN KEY (gearId) REFERENCES gear(gearId),
    FOREIGN KEY (userId) REFERENCES users(userId)
);
INSERT INTO user_gear_registrations (gearId, userId) VALUES
    (1, 1),
    (2, 1),
    (3, 1),
    (4, 1),
    (5, 1),
    (6, 1),
    (7, 1),
    (8, 1),
    (9, 1),
    (10, 1),
    (11, 1),
    (12, 1),
    (13, 1),
    (14, 1),
    (15, 1),
    (16, 1),
    (17, 1),
    (18, 1),
    (19, 1);

INSERT INTO users (userUsername, userPassword, userName, userEmail) VALUES 
    ("Bateau", "$2a$10$X2BAOJFWXxAudCm9ShaHvucsdv1.dz3pdbBPf6bJerWs7YJB7KV9", "Mats Bøe Bergmann", "mats.bergmann@gmail.com");

INSERT INTO gear_top_category (topCategoryName) VALUES 
    ("Footwear"),
    ("Clothing"),
    ("Backpacks"),
    ("Navigation and Safety"),
    ("Shelter"),
    ("Sleeping Gear"),
    ("Cooking"),
    ("Hiking Accessories"),
    ("Emergency and Communication"),
    ("Apparel Accessories");

INSERT INTO gear_category (categoryName, categoryTopCategoryId) VALUES 
    ("Hiking boots", 1),
    ("Trail shoes", 1),
    ("Hiking socks", 1),
    ("Gaiters", 1),
    ("Moisture-wicking base layers", 2),
    ("Moisture-wicking shirts", 2),
    ("Hiking pants", 2),
    ("shorts", 2),
    ("Insulation", 2),
    ("Rain jacket", 2),
    ("Rain pants", 2),
    ("Hat", 2),
    ("Sun hat", 2),
    ("Beanie", 2),
    ("Gloves", 2),
    ("Hiking backpack", 3),
    ("Daypack", 3),
    ("Hydration backpack", 3),
    ("Water bottle", 3),
    ("Maps", 4),
    ("Compass", 4),
    ("GPS", 4),
    ("Smartphone", 4),
    ("Whistle", 4),
    ("First aid kit", 4),
    ("Multi-tool", 4),
    ("knife", 4),
    ("Tent", 5),
    ("Footprint", 5),
    ("Tarp", 5),
    ("Emergency space blanket", 5),
    ("Bivy sack", 5),
    ("Ground tarp", 5),
    ("Sleeping bag", 6),
    ("Duvet", 6),
    ("Sleeping pad", 6),
    ("Air mattress", 6),
    ("Pillow", 6),
    ("Stove", 7),
    ("Fuel", 7),
    ("Cookware Pot", 7),
    ("Pan", 7),
    ("Utensils", 7),
    ("Lightweight food", 7),
    ("Water purification system", 7),
    ("Bear canister", 7),
    ("Trekking poles", 8),
    ("Sunglasses", 8),
    ("Sunscreen", 8),
    ("Insect repellent", 8),
    ("Headlamp", 8),
    ("Flashlight", 8),
    ("Batteries", 8),
    ("Camera", 8),
    ("Satellite communicator", 9),
    ("Personal Locator Beacon", 9),
    ("Two-way radios", 9),
    ("Bandana", 10),
    ("Buff", 10),
    ("Neck gaiter", 10),
    ("gloves", 10);


INSERT INTO manufacture (manufactureId, manufactureName) VALUES
    (1, "The North Face"),
    (2, "Patagonia"),
    (3, "Columbia Sportswear"),
    (4, "Arc'teryx"),
    (5, "Salomon"),
    (6, "Outdoor Research"),
    (7, "Marmot"),
    (8, "Black Diamond Equipment"),
    (9, "Osprey"),
    (10, "Gregory"),
    (11, "Deuter"),
    (12, "Kelty"),
    (13, "MSR (Mountain Safety Research)"),
    (14, "Petzl"),
    (15, "Merrell"),
    (16, "Keen"),
    (17, "Vasque"),
    (18, "La Sportiva"),
    (19, "Scarpa"),
    (20, "Mammut"),
    (21, "Hilleberg"),
    (22, "Big Agnes"),
    (23, "Therm-a-Rest"),
    (24, "REI Co-op"),
    (25, "Gossamer Gear"),
    (26, "Granite Gear"),
    (27, "Sea to Summit"),
    (28, "Rab"),
    (29, "Montane"),
    (30, "Fjällräven"),
    (31, "Hoka One One"),
    (32, "Oboz"),
    (33, "Altra"),
    (34, "Inov-8"),
    (35, "Lowa"),
    (36, "Exped"),
    (37, "Hyperlite Mountain Gear"),
    (38, "NEMO Equipment"),
    (39, "Western Mountaineering"),
    (40, "MontBell"),
    (41, "Garmont"),
    (42, "Salewa"),
    (43, "ORTOVOX"),
    (44, "Snow Peak"),
    (45, "Cotopaxi"),
    (46, "Klymit"),
    (47, "Blackyak"),
    (48, "Zamberlan"),
    (49, "Norrøna"),
    (50, "Devold"),
    (51, "Sweet Protection"),
    (52, "Lundhags"),
    (53, "Haglöfs"),
    (54, "Millet"),
    (55, "Vaude"),
    (56, "Wild Country"),
    (57, "Grivel"),
    (58, "CAMP"),
    (59, "Edelrid"),
    (60, "Sterling Rope"),
    (61, "BlueWater Ropes"),
    (62, "Five Ten"),
    (63, "Evolv"),
    (64, "Metolius Climbing"),
    (65, "Beal"),
    (66, "Maxim Ropes"),
    (67, "Trango"),
    (68, "Edelweiss"),
    (69, "Misty Mountain"),
    (70, "Camp USA"),
    (71, "Cassin"),
    (72, "DMM"),
    (73, "Houdini"),
    (74, "Didriksons"),
    (75, "Helly Hansen"),
    (76, "Bach"),
    (77, "Peak Performance"),
    (78, "Arctix"),
    (79, "Ulvang"),
    (80, "66°North"),
    (81, "Hestra"),
    (82, "Bula"),
    (83, "O'Neill"),
    (84, "Kari Traa"),
    (85, "Dale of Norway"),
    (86, "Icebreaker"),
    (87, "Trangia"),
    (88, "Bergans"),
    (89, "Crispi");

INSERT INTO gear (gearTopCategoryId,gearCategoryId,gearManufactureId,gearName,gearWeight,gearHeight,gearLength,gearWidth,gearStatus) VALUES
    (1, 1, 1, 'Salomon X Ultra 4 Mid GTX', 1000, 100, 300, 150, TRUE),
    (1, 1, 2, 'Merrell Moab 2 Vent', 500, 50, 150, 100, TRUE),
    (1, 1, 3, 'Altra Lone Peak 6', 750, 75, 225, 125, TRUE),
    (1, 2, 4, 'Prana Zion Stretch Pant', 500, 100, 75, 50, TRUE),
    (1, 2, 5, 'Patagonia Capilene Cool Trail Shirt', 250, 50, 50, 25, TRUE),
    (1, 2, 6, 'Arc teryx Cerium LT Hoody', 500, 100, 75, 50, TRUE),
    (1, 3, 7, 'Gregory Baltoro 85', 2000, 600, 300, 150, TRUE),
    (1, 3, 8, 'Deuter Aircontact Pro', 2100, 650, 310, 160, TRUE),
    (1, 4, 9, 'National Geographic Trails Illustrated Maps', 100, 25, 15, 10, TRUE),
    (1, 4, 10, 'Suunto A-3 Compass', 50, 25, 15, 10, TRUE),
    (1, 4, 11, 'Garmin Fenix 7 GPS Watch', 100, 25, 15, 10, TRUE),
    (1, 5, 12, 'MSR Elixir 2', 2100, 310, 200, 160, TRUE),
    (1, 6, 13, 'REI Co-op Stratus Sleeping Bag', 1100, 210, 160, 110, TRUE),
    (1, 7, 14, 'Jetboil Flash', 500, 100, 75, 50, TRUE),
    (1, 8, 15, 'Katadyn BeFree 3L Water Filter', 300, 55, 140, 75, TRUE),
    (1, 9, 16, 'Adventure Medical Kits Ultralight & Watertight 10 First Aid Kit', 600, 120, 85, 60, TRUE),
    (1, 10, 17, 'Nite Ize SpotLit Dog Tag Light', 50, 25, 15, 10, TRUE),
    (1, 10, 18, 'Petzl Activa Headlamp', 110, 25, 15, 10, TRUE),
    (2, 1, 19, 'La Sportiva TX5 GTX', 800, 150, 250, 130, TRUE),
    (2, 1, 20, 'Hoka One One Speedgoat 5', 700, 140, 240, 120, TRUE),
    (1, 1, 21, 'La Sportiva TX5 GTX', 800, 150, 250, 130, TRUE),
    (1, 1, 22, 'Hoka One One Speedgoat 5', 700, 140, 240, 120, TRUE),
    (1, 1, 23, 'Altra Torin 6 Plush', 900, 160, 260, 140, TRUE),
    (1, 1, 24, 'Saucony Peregrine 12', 650, 130, 230, 110, TRUE),
    (1, 1, 25, 'Salomon Speedcross 6', 700, 140, 240, 120, TRUE),
    (1, 2, 26, 'Darn Tough Vermont Hiker Micro Crew Socks', 100, 10, 10, 5, TRUE),
    (1, 2, 27, 'Smartwool PhD Outdoor Light Crew Socks', 110, 10, 11, 5, TRUE),
    (1, 2, 28, 'Icebreaker Merino Wool Cool-Lite Hike+ Crew Socks', 120, 11, 12, 6, TRUE),
    (1, 2, 29, 'Injinji Trail Midweight Crew Socks', 130, 12, 13, 6, TRUE),
    (1, 2, 30, 'Woolrich Mens Wool Boot Liner Socks', 140, 13, 14, 7, TRUE),
    (2, 3, 31, 'Prana Brion Pant', 600, 110, 80, 55, TRUE),
    (2, 3, 32, 'Patagonia Quandary Pants', 700, 120, 85, 60, TRUE),
    (2, 3, 33, 'Arcteryx Gamma AR Pant', 800, 130, 90, 65, TRUE),
    (2, 3, 34, 'REI Co-op Activator Pants', 900, 140, 95, 70, TRUE),
    (2, 3, 35, 'Columbia Silver Ridge Cargo Pants', 1000, 150, 100, 75, TRUE),
    (3, 4, 36, 'Osprey Atmos AG 65', 2000, 600, 300, 150, TRUE),
    (3, 4, 37, 'Gregory Baltoro 85', 2100, 650, 310, 160, TRUE),
    (3, 4, 38, 'Deuter Aircontact Pro', 2200, 700, 320, 170, TRUE),
    (3, 4, 39, 'REI Co-op Trail 45', 1900, 550, 290, 145, TRUE),
    (3, 4, 40, 'Kelty Coyote 65', 1800, 500, 280, 140, TRUE),
    (1, 1, 1, 'Salomon X Ultra 4 Mid GTX', 1000, 100, 300, 150, TRUE),
    (1, 1, 2, 'Merrell Moab 2 Vent', 500, 50, 150, 100, TRUE),
    (1, 1, 3, 'Altra Lone Peak 6', 750, 75, 225, 125, TRUE),
    (2, 2, 4, 'Osprey Atmos AG 65', 2000, 600, 300, 150, TRUE),
    (2, 2, 5, 'REI Co-op Magma 15', 1000, 200, 150, 100, TRUE),
    (2, 2, 6, 'Therm-a-Rest NeoAir XLite', 500, 100, 75, 50, TRUE),
    (3, 3, 7, 'Big Agnes Copper Spur HV UL 2', 2000, 300, 200, 150, TRUE),
    (3, 3, 8, 'MSR PocketRocket 2', 500, 100, 75, 50, TRUE),
    (3, 3, 9, 'Sawyer Mini Water Filter', 250, 50, 50, 25, TRUE),
    (4, 4, 10, 'Prana Zion Stretch Pant', 500, 100, 75, 50, TRUE),
    (4, 4, 11, 'Patagonia Capilene Cool Trail Shirt', 250, 50, 50, 25, TRUE),
    (4, 4, 12, 'Arc teryx Cerium LT Hoody', 500, 100, 75, 50, TRUE),
    (5, 5, 13, 'National Geographic Trails Illustrated Maps', 100, 25, 15, 10, TRUE),
    (5, 5, 14, 'Suunto A-3 Compass', 50, 25, 15, 10, TRUE),
    (5, 5, 15, 'Garmin Fenix 7 GPS Watch', 100, 25, 15, 10, TRUE),
    (6, 6, 16, 'Adventure Medical Kits Ultralight 7 First Aid Kit', 500, 100, 75, 50, TRUE),
    (6, 6, 17, 'Supergoop! Unseen Sunscreen SPF 40', 250, 50, 50, 25, TRUE),
    (6, 6, 18, 'Sawyer Picaridin Insect Repellent', 100, 25, 15, 10, TRUE),
    (6, 6, 19, 'Fox 40 Classic Whistle', 50, 25, 15, 10, TRUE),
    (6, 6, 20, 'Petzl Tikka Headlamp', 100, 25, 15, 10, TRUE),
    (1, 1, 21, 'La Sportiva TX5 GTX', 800, 150, 250, 130, TRUE),
    (1, 1, 22, 'Hoka One One Speedgoat 5', 700, 140, 240, 120, TRUE),
    (1, 1, 23, 'Altra Torin 6 Plush', 900, 160, 260, 140, TRUE),
    (2, 2, 24, 'Osprey Ariel AG 65', 2100, 650, 310, 160, TRUE),
    (2, 2, 25, 'REI Co-op Stratus Sleeping Bag', 1100, 210, 160, 110, TRUE),
    (2, 2, 26, 'Therm-a-Rest NeoAir XTherm', 600, 110, 76, 51, TRUE),
    (3, 3, 27, 'MSR Elixir 2', 2100, 310, 200, 160, TRUE),
    (3, 3, 28, 'Jetboil Flash', 500, 100, 75, 50, TRUE),
    (3, 3, 29, 'Katadyn BeFree 3L Water Filter', 300, 55, 140, 75, TRUE),
    (4, 4, 30, 'Prana Brion Pant', 600, 110, 80, 55, TRUE),
    (4, 4, 31, 'Patagonia Capilene Cool Daily Graphic Shirt', 300, 60, 60, 40, TRUE),
    (4, 4, 32, 'Arc teryx Gamma AR Pant', 700, 120, 85, 60, TRUE),
    (5, 5, 33, 'Gaia GPS App', 100, 25, 15, 10, TRUE),
    (5, 5, 34, 'Silva Expedition 4 Compass', 60, 25, 15, 10, TRUE),
    (5, 5, 35, 'Garmin inReach Mini', 150, 30, 30, 20, TRUE),
    (6, 6, 36, 'Adventure Medical Kits Ultralight & Watertight 10 First Aid Kit', 600, 120, 85, 60, TRUE),
    (6, 6, 37, 'Supergoop! Unseen Sunscreen SPF 50', 300, 60, 60, 40, TRUE),
    (6, 6, 38, 'REI Co-op Insect Repellent Spray', 200, 50, 50, 35, TRUE),
    (6, 6, 39, 'Nite Ize SpotLit Dog Tag Light', 50, 25, 15, 10, TRUE),
    (6, 6, 40, 'Petzl Activa Headlamp', 110, 25, 15, 10, TRUE);